.PHONY: build test test-coverage test-short test-integration test-unit docker clean

BUILD_OUTPUT=dist/llmsee

build:
	CGO_ENABLED=0 GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) go build -o $(BUILD_OUTPUT)

test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-short:
	go test -v -short ./...

test-integration:
	go test -v -run TestIntegration ./...

test-unit:
	go test -v -run "^Test[^I]" ./...

docker:
	docker build -t llmsee .

clean:
	rm -f $(BUILD_OUTPUT) coverage.out coverage.html
