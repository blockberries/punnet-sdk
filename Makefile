.PHONY: all build test test-race lint clean install-tools generate bench bench-compare

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
	@go install golang.org/x/perf/cmd/benchstat@latest

mod-tidy:
	@echo "Tidying go.mod..."
	@go mod tidy

mod-download:
	@echo "Downloading dependencies..."
	@go mod download

generate:
	@echo "Generating code from Cramberry schemas..."
	@echo "TODO: Install cramberry compiler and generate Go code"
	@echo "cramberry generate -lang go -out ./types/generated ./schema/types.cram"
	@echo "cramberry generate -lang go -out ./modules/auth/generated ./schema/auth.cram"
	@echo "cramberry generate -lang go -out ./modules/bank/generated ./schema/bank.cram"
	@echo "cramberry generate -lang go -out ./modules/staking/generated ./schema/staking.cram"

clean-generated:
	@echo "Cleaning generated files..."
	@rm -rf ./types/generated
	@rm -rf ./modules/auth/generated
	@rm -rf ./modules/bank/generated
	@rm -rf ./modules/staking/generated

bench:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

bench-save:
	@echo "Running benchmarks and saving to baseline..."
	@go test -bench=. -benchmem -count=5 ./... 2>/dev/null | tee benchmarks/baseline.txt

bench-compare:
	@echo "Running benchmarks and comparing to baseline..."
	@go test -bench=. -benchmem -count=5 ./... 2>/dev/null > /tmp/bench-new.txt
	@benchstat benchmarks/baseline.txt /tmp/bench-new.txt
