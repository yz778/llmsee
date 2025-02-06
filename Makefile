.PHONY: build

BUILD_OUTPUT=dist/llmsee

build:
	CGO_ENABLED=0 GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) go build -o $(BUILD_OUTPUT)

docker:
	docker build -t llmsee .
