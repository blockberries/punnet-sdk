.PHONY: all build test test-race lint clean install-tools

all: build test

build:
	@echo "Building punnet-sdk..."
	@go build ./...

test:
	@echo "Running tests..."
	@go test ./...

test-race:
	@echo "Running tests with race detector..."
	@go test -race -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

lint:
	@echo "Running linter..."
	@golangci-lint run

clean:
	@echo "Cleaning..."
	@rm -f coverage.out coverage.html
	@go clean ./...

install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

mod-tidy:
	@echo "Tidying go.mod..."
	@go mod tidy

mod-download:
	@echo "Downloading dependencies..."
	@go mod download
