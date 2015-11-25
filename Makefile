.DEFAULT_GOAL=provision
.PHONY: build test get run tags

ensure_vendor:
	mkdir -pv vendor

clean:
	rm -rf ./vendor
	go clean .

format:
	go fmt .
	gofmt -s -w ./transforms/

vet:
	go vet .

generate:
	go generate -x

get: clean ensure_vendor
	git clone --depth=1 https://github.com/aws/aws-sdk-go ./vendor/github.com/aws/aws-sdk-go
	rm -rf ./src/main/vendor/github.com/aws/aws-sdk-go/.git
	git clone --depth=1 https://github.com/vaughan0/go-ini ./vendor/github.com/vaughan0/go-ini
	rm -rf ./src/main/vendor/github.com/vaughan0/go-ini/.git
	git clone --depth=1 https://github.com/Sirupsen/logrus ./vendor/github.com/Sirupsen/logrus
	rm -rf ./src/main/vendor/github.com/Sirupsen/logrus/.git
	git clone --depth=1 https://github.com/voxelbrain/goptions ./vendor/github.com/voxelbrain/goptions
	rm -rf ./src/main/vendor/github.com/voxelbrain/goptions/.git
	git clone --depth=1 https://github.com/mweagle/esc ./vendor/github.com/mweagle/esc
	rm -rf ./src/main/vendor/github.com/mweagle/esc/.git
	git clone --depth=1 https://github.com/mweagle/Sparta ./vendor/github.com/mweagle/Sparta
	rm -rf ./src/main/vendor/github.com/mweagle/Sparta/.git

build: get format vet generate
	GO15VENDOREXPERIMENT=1 go build .

test: build
	GO15VENDOREXPERIMENT=1 go test ./test/...

tags:
	gotags -tag-relative=true -R=true -sort=true -f="tags" -fields=+l .

provision:
	go run application.go --level debug provision --s3Bucket $(S3_BUCKET)

describe:
	go run application.go --level info describe --out ./graph.html
