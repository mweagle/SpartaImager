package main

// Embed the shields
//go:generate go run ./vendor/github.com/mjibson/esc/main.go -o ./assets/CONSTANTS.go -pkg assets ./resources
//

import (
	"SpartaImager/transforms"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	sparta "github.com/mweagle/Sparta"
	"net/http"
	"os"
	"strings"
)

////////////////////////////////////////////////////////////////////////////////
func paramVal(keyName string, defaultValue string) string {
	value := os.Getenv(keyName)
	if "" == value {
		value = defaultValue
	}
	return value
}

var S3_EVENT_BROADCASTER_BUCKET = paramVal("S3_TEST_BUCKET", "arn:aws:s3:::PublicS3Bucket")
var S3_SOURCE_BUCKET = paramVal("S3_BUCKET", "arn:aws:s3:::MyS3Bucket")

type bucketInfo struct {
	Name string `json:"name"`
}

type objectInfo struct {
	Key string `json:"key"`
}

type s3EventInfo struct {
	Bucket bucketInfo `json:"bucket"`
	Object objectInfo `json:"object"`
}

type eventRecord struct {
	Region    string `json:"awsRegion"`
	EventName string `json:"eventName"`
	EventTime string `json:"eventTime"`

	S3 s3EventInfo `json:"s3"`
}

type s3Event struct {
	Records []eventRecord
}

// Returns an AWS Session (https://github.com/aws/aws-sdk-go/wiki/Getting-Started-Configuration)
// object that attaches a debug level handler to all AWS requests from services
// sharing the session value.
func awsSession(logger *logrus.Logger) *session.Session {
	sess := session.New()
	sess.Handlers.Send.PushFront(func(r *request.Request) {
		logger.WithFields(logrus.Fields{
			"Service":   r.ClientInfo.ServiceName,
			"Operation": r.Operation.Name,
			"Method":    r.Operation.HTTPMethod,
			"Path":      r.Operation.HTTPPath,
			"Payload":   r.Params,
		}).Info("AWS Request")
	})
	return sess
}

const transformPrefix = "xformed_"

func stampImage(bucket string, key string, logger *logrus.Logger) error {

	// Only transform if the key doesn't have the _xformed part
	if !strings.Contains(key, transformPrefix) {
		awsSession := awsSession(logger)
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

func transformImage(event *json.RawMessage, context *sparta.LambdaContext, w *http.ResponseWriter, logger *logrus.Logger) {
	logger.WithFields(logrus.Fields{
		"RequestID": context.AWSRequestId,
	}).Info("Request received")

	var lambdaEvent s3Event
	err := json.Unmarshal([]byte(*event), &lambdaEvent)
	if err != nil {
		logger.Error("Failed to unmarshal event data: ", err.Error())
		http.Error(*w, err.Error(), http.StatusInternalServerError)
	}

	logger.WithFields(logrus.Fields{
		"S3Event": lambdaEvent,
	}).Info("S3 Notification")

	for _, eachRecord := range lambdaEvent.Records {
		err = nil
		// What happened?
		switch eachRecord.EventName {
		case "ObjectCreated:Put":
			{
				err = stampImage(eachRecord.S3.Bucket.Name, eachRecord.S3.Object.Key, logger)
			}
		case "s3:ObjectRemoved:Delete":
			{
				deleteKey := fmt.Sprintf("%s%s", transformPrefix, eachRecord.S3.Object.Key)
				awsSession := awsSession(logger)
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
			http.Error(*w, err.Error(), http.StatusInternalServerError)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// Return the *[]sparta.LambdaAWSInfo slice
//
func imagerFunctions() []*sparta.LambdaAWSInfo {

	// Provision an IAM::Role as part of this application
	var iamRole = sparta.IAMRoleDefinition{}

	// Setup the ARN that includes all child keys
	resourceArn := fmt.Sprintf("%s/*", S3_EVENT_BROADCASTER_BUCKET)
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
			SourceArn: S3_EVENT_BROADCASTER_BUCKET,
		},
		Events: []string{"s3:ObjectCreated:*", "s3:ObjectRemoved:*"},
	})
	lambdaFunctions = append(lambdaFunctions, lambdaFn)
	return lambdaFunctions
}

func main() {
	sparta.Main("SpartaImager", "This is a sample Sparta application", imagerFunctions())
}
