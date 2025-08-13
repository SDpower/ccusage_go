.PHONY: build clean test lint install build-all

# Default target
build:
	go build -o bin/ccusage ./cmd/ccusage

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
benchmark:
	go test -bench=. -benchmem ./...

# Lint code
lint:
	golangci-lint run

# Install to GOPATH
install:
	go install ./cmd/ccusage

# Multi-platform build
build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/ccusage-linux-amd64 ./cmd/ccusage
	GOOS=darwin GOARCH=amd64 go build -o bin/ccusage-darwin-amd64 ./cmd/ccusage
	GOOS=darwin GOARCH=arm64 go build -o bin/ccusage-darwin-arm64 ./cmd/ccusage
	GOOS=windows GOARCH=amd64 go build -o bin/ccusage-windows-amd64.exe ./cmd/ccusage

# Run go mod tidy
tidy:
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# All quality checks
check: fmt vet test

# Development build with debug info
dev:
	go build -gcflags="all=-N -l" -o bin/ccusage-dev ./cmd/ccusage