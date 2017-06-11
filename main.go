package main

// Embed the Sparta Helmets for the watermark
//go:generate go run $GOPATH/src/github.com/mjibson/esc/main.go -o ./assets/CONSTANTS.go -pkg assets ./resources
//

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mweagle/SpartaImager/transforms"

	spartaS3 "github.com/mweagle/Sparta/aws/s3"

	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
)

////////////////////////////////////////////////////////////////////////////////
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

var s3EventBroadcasterBucket = s3ARNParamValue("SPARTA_S3_TEST_BUCKET", "arn:aws:s3:::PublicS3Bucket")

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
		transformed, err := transforms.StampImage(result.Body, logger)
		if err != nil {
			return err
		}
		// Put the encoded image to a byte buffer, then wrap a reader around it.
		uploadResult, err := svc.PutObject(&s3.PutObjectInput{
			Body:   transformed,
			Bucket: aws.String(bucket),
			Key:    aws.String(fmt.Sprintf("%s%s", transformPrefix, key)),
		})
		if err != nil {
			return err
		}
		logger.WithFields(logrus.Fields{
			"Transformed": uploadResult,
		}).Info("Image transformed")

	} else {
		logger.Info("File already transformed")
	}
	return nil
}

func transformImage(event *json.RawMessage,
	context *sparta.LambdaContext,
	w http.ResponseWriter,
	logger *logrus.Logger) {

	logger.WithFields(logrus.Fields{
		"RequestID": context.AWSRequestID,
		"Event":     string(*event),
	}).Info("Request received 👍")

	var lambdaEvent spartaS3.Event
	err := json.Unmarshal([]byte(*event), &lambdaEvent)
	if err != nil {
		logger.Error("Failed to unmarshal event data: ", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	logger.WithFields(logrus.Fields{
		"S3Event": fmt.Sprintf("%+v", lambdaEvent),
	}).Info("S3 Notification")

	for _, eachRecord := range lambdaEvent.Records {
		err = nil
		// What happened?
		switch eachRecord.EventName {
		case "ObjectCreated:Put":
			{
				// Make sure the Name and Key are URL decoded. Spaces are + encoded
				unescapedKeyName, _ := url.QueryUnescape(eachRecord.S3.Bucket.Name)
				unescapedBucketName, _ := url.QueryUnescape(eachRecord.S3.Object.Key)
				err = stampImage(unescapedKeyName, unescapedBucketName, logger)
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
				resp, err := svc.DeleteObject(params)
				logger.WithFields(logrus.Fields{
					"Response": resp,
					"Error":    err,
				}).Info("Deleted object")
			}
		default:
			{
				logger.Info("Unsupported event: ", eachRecord.EventName)
			}
		}

		//
		if err != nil {
			logger.Error("Failed to process event: ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func s3ItemInfo(event *json.RawMessage, context *sparta.LambdaContext, w http.ResponseWriter, logger *logrus.Logger) {
	logger.WithFields(logrus.Fields{
		"RequestID": context.AWSRequestID,
		"Event":     string(*event),
	}).Info("Request received")

	var lambdaEvent sparta.APIGatewayLambdaJSONEvent
	err := json.Unmarshal([]byte(*event), &lambdaEvent)
	if err != nil {
		logger.Error("Failed to unmarshal event data: ", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(lambdaEvent.QueryParams["bucketName"]),
		Key:    aws.String(lambdaEvent.QueryParams["keyName"]),
	}

	awsSession := spartaAWS.NewSession(logger)
	svc := s3.New(awsSession)
	result, err := svc.GetObject(getObjectInput)
	if nil != err {
		logger.Error("Failed to process event: ", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	presignedReq, _ := svc.GetObjectRequest(getObjectInput)
	url, err := presignedReq.Presign(5 * time.Minute)
	if nil != err {
		logger.Error("Failed to process event: ", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	httpResponse := map[string]interface{}{
		"S3":  result,
		"URL": url,
	}

	responseBody, err := json.Marshal(httpResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(responseBody))
	}
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
		MemorySize:  128,
		Timeout:     30,
	}
	lambdaFn := sparta.NewLambda(iamRole, transformImage, transformOptions)

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
	s3ItemInfoOptions := &sparta.LambdaFunctionOptions{
		Description: "Get information about an item in S3 via querystring params",
		MemorySize:  128,
		Timeout:     10,
	}
	var iamDynamicRole = sparta.IAMRoleDefinition{}
	iamDynamicRole.Privileges = append(iamDynamicRole.Privileges, sparta.IAMRolePrivilege{
		Actions:  []string{"s3:GetObject"},
		Resource: resourceArn,
	})
	s3ItemInfoLambdaFn := sparta.NewLambda(iamDynamicRole, s3ItemInfo, s3ItemInfoOptions)
	// Register the function with the API Gateway
	apiGatewayResource, _ := api.NewResource("/info", s3ItemInfoLambdaFn)
	method, err := apiGatewayResource.NewMethod("GET", http.StatusOK)
	if err != nil {
		return nil, err
	}
	// Whitelist query string params
	method.Parameters["method.request.querystring.keyName"] = true
	method.Parameters["method.request.querystring.bucketName"] = true
	lambdaFunctions = append(lambdaFunctions, s3ItemInfoLambdaFn)

	return lambdaFunctions, nil
}

func main() {
	apiStage := sparta.NewStage("v1")
	apiGateway := sparta.NewAPIGateway("SpartaImagerAPI", apiStage)
	apiGateway.CORSEnabled = true
	funcs, err := imagerFunctions(nil)
	if err == nil {
		sparta.Main("SpartaImagerRoundtrip", "This is a sample Sparta application", funcs, apiGateway, nil)
	}
}
