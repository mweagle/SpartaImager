package main

// Embed the Sparta Helmets for the watermark
//go:generate go run $GOPATH/src/github.com/mjibson/esc/main.go -o ./assets/CONSTANTS.go -pkg assets ./resources
//

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	awsLambdaEvents "github.com/aws/aws-lambda-go/events"
	awsLambdaContext "github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
	spartaAPIGateway "github.com/mweagle/Sparta/aws/apigateway"
	spartaCF "github.com/mweagle/Sparta/aws/cloudformation"
	spartaEvents "github.com/mweagle/Sparta/aws/events"
	"github.com/mweagle/SpartaImager/transforms"
	"github.com/sirupsen/logrus"
)

////////////////////////////////////////////////////////////////////////////////

type transformedResponse struct {
	Bucket string
	Key    string
}

type itemInfoResponse struct {
	S3  *s3.GetObjectOutput
	URL string
}

func s3ARNParamValue(keyName string, defaultValue string) string {
	value := os.Getenv(keyName)
	if "" == value {
		value = defaultValue
	}
	// If it doesn't look like an S3 ARN, add that...
	if !strings.Contains(value, "arn:aws:s3:::") {
		value = fmt.Sprintf("arn:aws:s3:::%s", value)
	}
	return value
}

var s3EventBroadcasterBucket = s3ARNParamValue("SPARTA_S3_TEST_BUCKET",
	"arn:aws:s3:::PublicS3Bucket")

const transformPrefix = "xformed_"

func stampImage(bucket string, key string, logger *logrus.Logger) error {

	// Only transform if the key doesn't have the _xformed part
	if !strings.Contains(key, transformPrefix) {
		awsSession := spartaAWS.NewSession(logger)
		svc := s3.New(awsSession)
		result, err := svc.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if nil != err {
			return err
		}
		defer result.Body.Close()

		transformed, transformedErr := transforms.StampImage(result.Body, logger)
		if transformedErr != nil {
			return transformedErr
		}
		// Put the encoded image to a byte buffer, then wrap a reader around it.
		_, uploadResultErr := svc.PutObject(&s3.PutObjectInput{
			Body:   transformed,
			Bucket: aws.String(bucket),
			Key:    aws.String(fmt.Sprintf("%s%s", transformPrefix, key)),
		})
		if uploadResultErr != nil {
			return uploadResultErr
		}
	} else {
		logger.Info("File already transformed")
	}
	return nil
}

func transformImage(ctx context.Context,
	event awsLambdaEvents.S3Event) (*spartaAPIGateway.Response, error) {
	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	lambdaContext, _ := awsLambdaContext.FromContext(ctx)
	logger.WithFields(logrus.Fields{
		"RequestID":   lambdaContext.AwsRequestID,
		"RecordCount": len(event.Records),
	}).Info("Request received 👍")

	responses := make([]transformedResponse, 0)

	for _, eachRecord := range event.Records {
		// What happened?
		switch eachRecord.EventName {
		case "ObjectCreated:Put":
			{
				// Make sure the Name and Key are URL decoded. Spaces are + encoded
				unescapedKeyName, _ := url.QueryUnescape(eachRecord.S3.Bucket.Name)
				unescapedBucketName, _ := url.QueryUnescape(eachRecord.S3.Object.Key)
				stampErr := stampImage(unescapedKeyName, unescapedBucketName, logger)
				if stampErr != nil {
					return nil, spartaAPIGateway.NewErrorResponse(http.StatusInternalServerError, stampErr)
				}

				logger.WithFields(logrus.Fields{
					"item": eachRecord.S3.Object.Key,
				}).Info("Image stamped")

				responses = append(responses, transformedResponse{
					Bucket: unescapedKeyName,
					Key:    unescapedKeyName,
				})
			}
		case "s3:ObjectRemoved:Delete":
			{
				deleteKey := fmt.Sprintf("%s%s", transformPrefix, eachRecord.S3.Object.Key)
				awsSession := spartaAWS.NewSession(logger)
				svc := s3.New(awsSession)

				params := &s3.DeleteObjectInput{
					Bucket: aws.String(eachRecord.S3.Bucket.Name),
					Key:    aws.String(deleteKey),
				}
				deleteObj, deleteObjErr := svc.DeleteObject(params)
				if deleteObjErr != nil {
					logger.WithFields(logrus.Fields{
						"Response": deleteObj,
					}).Info("Deleted object")
				}
			}
		default:
			{
				logger.Info("Unsupported event: ", eachRecord.EventName)
			}
		}
	}
	return spartaAPIGateway.NewResponse(http.StatusOK, responses), nil
}

func s3ItemInfo(ctx context.Context,
	apigRequest spartaEvents.APIGatewayRequest) (*spartaAPIGateway.Response, error) {

	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	lambdaContext, _ := awsLambdaContext.FromContext(ctx)

	logger.WithFields(logrus.Fields{
		"RequestID": lambdaContext.AwsRequestID,
	}).Info("Request received")

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(apigRequest.QueryParams["bucketName"]),
		Key:    aws.String(apigRequest.QueryParams["keyName"]),
	}

	awsSession := spartaAWS.NewSession(logger)
	svc := s3.New(awsSession)
	result, err := svc.GetObject(getObjectInput)
	if nil != err {
		return spartaAPIGateway.NewResponse(http.StatusNotFound, map[string]string{
			"error": err.Error(),
		}), nil
	}
	presignedReq, _ := svc.GetObjectRequest(getObjectInput)
	url, err := presignedReq.Presign(5 * time.Minute)
	if nil != err {
		return nil, err
	}
	return spartaAPIGateway.NewResponse(http.StatusOK, &itemInfoResponse{
		S3:  result,
		URL: url,
	}), nil
}

////////////////////////////////////////////////////////////////////////////////
// Return the *[]sparta.LambdaAWSInfo slice
//
func imagerFunctions(api *sparta.API) ([]*sparta.LambdaAWSInfo, error) {

	//////////////////////////////////////////////////////////////////////////////
	// 1 - Lambda function that listens to S3 events and stamps images
	//////////////////////////////////////////////////////////////////////////////
	// Provision an IAM::Role as part of this application
	var iamRole = sparta.IAMRoleDefinition{}

	// Setup the ARN that includes all child keys
	resourceArn := fmt.Sprintf("%s/*", s3EventBroadcasterBucket)
	iamRole.Privileges = append(iamRole.Privileges, sparta.IAMRolePrivilege{
		Actions: []string{"s3:GetObject",
			"s3:PutObject",
		},
		Resource: resourceArn,
	})
	var lambdaFunctions []*sparta.LambdaAWSInfo

	// The default timeout is 3 seconds - increase that to 30 seconds s.t. the
	// transform lambda doesn't fail early.
	transformOptions := &sparta.LambdaFunctionOptions{
		Description: "Stamp assets in S3",
		MemorySize:  256,
		Timeout:     10,
	}
	lambdaFn, _ := sparta.NewAWSLambda(sparta.LambdaName(transformImage),
		transformImage,
		iamRole)
	lambdaFn.Options = transformOptions

	//////////////////////////////////////////////////////////////////////////////
	// S3 configuration
	//
	lambdaFn.Permissions = append(lambdaFn.Permissions, sparta.S3Permission{
		BasePermission: sparta.BasePermission{
			SourceArn: s3EventBroadcasterBucket,
		},
		Events: []string{"s3:ObjectCreated:*", "s3:ObjectRemoved:*"},
	})
	lambdaFunctions = append(lambdaFunctions, lambdaFn)

	//////////////////////////////////////////////////////////////////////////////
	// 2 - Lambda function that allows for querying of S3 information
	//////////////////////////////////////////////////////////////////////////////
	var iamDynamicRole = sparta.IAMRoleDefinition{}
	iamDynamicRole.Privileges = append(iamDynamicRole.Privileges, sparta.IAMRolePrivilege{
		Actions:  []string{"s3:GetObject"},
		Resource: resourceArn,
	})

	s3ItemInfoLambdaFn := sparta.HandleAWSLambda(sparta.LambdaName(s3ItemInfo),
		s3ItemInfo,
		iamDynamicRole)
	s3ItemInfoLambdaFn.Options = &sparta.LambdaFunctionOptions{
		Description: "Get information about an item in S3 via querystring params",
		MemorySize:  128,
		Timeout:     10,
	}
	// Register the function with the API Gateway iff defined
	if api != nil {
		apiGatewayResource, _ := api.NewResource("/info", s3ItemInfoLambdaFn)
		method, err := apiGatewayResource.NewMethod("GET", http.StatusOK)
		if err != nil {
			return nil, err
		}
		// Whitelist query string params
		method.Parameters["method.request.querystring.keyName"] = true
		method.Parameters["method.request.querystring.bucketName"] = true
	}
	lambdaFunctions = append(lambdaFunctions, s3ItemInfoLambdaFn)
	return lambdaFunctions, nil
}

func main() {
	apiStage := sparta.NewStage("v1")
	apiGateway := sparta.NewAPIGateway("SpartaImagerAPI", apiStage)
	apiGateway.CORSEnabled = true
	funcs, err := imagerFunctions(apiGateway)
	stackName := spartaCF.UserScopedStackName("SpartaImager")

	if err == nil {
		sparta.Main(stackName,
			"This is a sample Sparta application",
			funcs,
			apiGateway,
			nil)
	}
}
