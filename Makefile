.DEFAULT_GOAL=provision
.PHONY: build test get run tags

format:
	go fmt .
	gofmt -s -w ./transforms/

vet:
	go vet .

generate:
	go generate -x

build: format vet generate
	go build .

explore:
	go run main.go --level info explore

test: build
	go test ./test/...

provision:
	go run main.go --level info provision --s3Bucket $(S3_BUCKET)

describe:
	go run main.go --level info describe --out ./graph.html --s3Bucket $(S3_BUCKET)

delete:
	go run main.go --level info delete
