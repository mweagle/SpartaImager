# SpartaImager

The canonical S3 event image watermarking #serverless application. This application shows how you can
use [Sparta](http://gosparta.io) to:
  - Provision a Lambda function and an API Gateway
  - Configure S3 event listeners
  - Provision an API Gateway that returns [pre-signed S3 URLs](http://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/s3-example-presigned-urls.html) for public download.
  - Access the transformed image

## Provision

```bash
S3_BUCKET=weagle SPARTA_S3_TEST_BUCKET=<S3_BUCKET_TO_USE_AS_EVENT_SOURCE> go run main.go provision --s3Bucket ${S3_BUCKET}
INFO[0000] ========================================
INFO[0000] Welcome to SpartaImager                       GoVersion=go1.8.3 LinkFlags= Option=provision SpartaVersion=0.12.0 UTC="2017-06-11T17:47:59Z"
INFO[0000] ========================================
INFO[0000] Provisioning service                          BuildID=f1cfa7da8c30f189ce43320299da619410c0001c CodePipelineTrigger= InPlaceUpdates=false NOOP=false Tags=
INFO[0000] Verifying IAM Lambda execution roles
INFO[0000] IAM roles verified                            Count=2
INFO[0000] Checking S3 versioning                        Bucket=weagle VersioningEnabled=true
INFO[0000] Running `go generate`
...
INFO[0054] Waiting for CloudFormation operation to complete
INFO[0068] Stack output                                  Description="API Gateway URL" Key=APIGatewayURL Value="https://hxkf6p61r7.execute-api.us-west-2.amazonaws.com/v1"
INFO[0068] Stack provisioned                             CreationTime=2017-06-11 16:44:50.619 +0000 UTC StackId="arn:aws:cloudformation:us-west-2:027159405834:stack/SpartaImager/496cf250-4ec5-11e7-88ca-50a686fc379a" StackName=SpartaImager

```

## Upload

To upload a file, use the S3 command line tool (or equivalent):

<div align="center"><img src="https://raw.githubusercontent.com/mweagle/SpartaImager/master/site/ben.jpg" />
</div>


```bash
SPARTA_S3_TEST_BUCKET=<S3_BUCKET_TO_USE_AS_EVENT_SOURCE> aws s3 cp ./site/ben.jpg s3://<S3_BUCKET_TO_USE_AS_EVENT_SOURCE>/ben.jpg
```

## Fetch PreSigned Download

Using the **APIGatewayURL** output referenced above, fetch information about the transformed item via the `/info` path. The transformed item name is the original filename with a `xformed_` prefix: `ben.jpg ==> xformed_ben.jpg`

Provide the following query arguments to the `/info` resource:
  * _bucketName_ : The name of the S3 bucket (`SPARTA_S3_TEST_BUCKET` value above)
  * _keyName_ : The name of the file to return metadata about. Eg: `xformed_ben.jpg`


```bash
curl "https://hxkf6p61r7.execute-api.us-west-2.amazonaws.com/v1/info?bucketName=<S3_BUCKET_TO_USE_AS_EVENT_SOURCE>&keyName=xformed_ben.jpg" | python -m json.tool
```
Or if you have [jq](https://stedolan.github.io/jq/) installed:

```bash
curl "https://hxkf6p61r7.execute-api.us-west-2.amazonaws.com/v1/info?bucketName=<S3_BUCKET_TO_USE_AS_EVENT_SOURCE>&keyName=xformed_ben.jpg" | jq .
```

```json
{
    "S3": {
        "AcceptRanges": "bytes",
        "Body": {},
        "CacheControl": null,
        "ContentDisposition": null,
        "ContentEncoding": null,
        "ContentLanguage": null,
        "ContentLength": 407954,
        "ContentRange": null,
        "ContentType": "image/jpeg",
        "DeleteMarker": null,
        "ETag": "\"9bb993ee2060ffeb0b752f9a345c04ec\"",
        "Expiration": null,
        "Expires": null,
        "LastModified": "2017-06-11T18:04:16Z",
        "Metadata": {},
        "MissingMeta": null,
        "PartsCount": null,
        "ReplicationStatus": null,
        "RequestCharged": null,
        "Restore": null,
        "SSECustomerAlgorithm": null,
        "SSECustomerKeyMD5": null,
        "SSEKMSKeyId": null,
        "ServerSideEncryption": null,
        "StorageClass": null,
        "TagCount": null,
        "VersionId": null,
        "WebsiteRedirectLocation": null
    },
    "URL": "https://<S3_BUCKET_TO_USE_AS_EVENT_SOURCE>.s3-us-west-2.amazonaws.com/ben.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=ASIAIRNCZ7DRLEX3NIRA%2F20170611%2Fus-west-2%2Fs3%2Faws4_request&X-Amz-Date=20170611T181726Z&X-Amz-Expires=300&X-Amz-Security-Token=FQoDYXdzEKr%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FwEaDNhDlEZvt8tXPoW%2BLSKZAj5EqEvplV5qlyzPe%2FFtFXy%2B9PhYbD%2FbXNYVjiFsUg4v7ivSbqz3%2FMxeFigMXCaYzgxoCk%2B26urq0ehgrERiT3JnKnmmtbthQlYp4c4lL9Bm3M8K8xDMBc%2F8XSe6b5LcCX6ZQ1Vh2sago0hPdRc4QSqdhMbpiUGssfSS4i3RdgUHhh4oQ9sPutKxp9yBCU1GDREdKsoOuURBSEWjKOo5SgV9MvRVHap8Y8ex1Iau3W0VXLaUPy4nLvHorZ0MpASPZyLaiBlTMea0IHreg9wUX4IekA5EcrsbhLkMUBF4aueT2KIZ4MfdSOAbof9oSik3mnFD5M75aAkk3iya7kWwYmy3hqqrt7b%2BAZWUhCqv5JYXhvkM1PAm4SEKKOXx9ckF&X-Amz-SignedHeaders=host&X-Amz-Signature=51a5d9a7fa1fd8ce16a0cbbc0913a4e87f58b105920f312227097b21ef910e15"
}
```

*NOTE* : The presigned `URL` will expire after 5 minutes.

## Download

Finally, using the `URL` value returned in the JSON response, download the transformed item:

```bash
curl "https://<S3_BUCKET_TO_USE_AS_EVENT_SOURCE>.s3-us-west-2.amazonaws.com/ben.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=ASIAIRNCZ7DRLEX3NIRA%2F20170611%2Fus-west-2%2Fs3%2Faws4_request&X-Amz-Date=20170611T181726Z&X-Amz-Expires=300&X-Amz-Security-Token=FQoDYXdzEKr%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FwEaDNhDlEZvt8tXPoW%2BLSKZAj5EqEvplV5qlyzPe%2FFtFXy%2B9PhYbD%2FbXNYVjiFsUg4v7ivSbqz3%2FMxeFigMXCaYzgxoCk%2B26urq0ehgrERiT3JnKnmmtbthQlYp4c4lL9Bm3M8K8xDMBc%2F8XSe6b5LcCX6ZQ1Vh2sago0hPdRc4QSqdhMbpiUGssfSS4i3RdgUHhh4oQ9sPutKxp9yBCU1GDREdKsoOuURBSEWjKOo5SgV9MvRVHap8Y8ex1Iau3W0VXLaUPy4nLvHorZ0MpASPZyLaiBlTMea0IHreg9wUX4IekA5EcrsbhLkMUBF4aueT2KIZ4MfdSOAbof9oSik3mnFD5M75aAkk3iya7kWwYmy3hqqrt7b%2BAZWUhCqv5JYXhvkM1PAm4SEKKOXx9ckF&X-Amz-SignedHeaders=host&X-Amz-Signature=51a5d9a7fa1fd8ce16a0cbbc0913a4e87f58b105920f312227097b21ef910e15" > xformed_ben.jpg
```


<div align="center"><img src="https://raw.githubusercontent.com/mweagle/SpartaImager/master/site/xformed_ben.jpg" />
</div>