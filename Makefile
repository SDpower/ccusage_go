.PHONY: build clean test lint install build-all

# Default target - static build with optimizations
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go ./cmd/ccusage

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

# Multi-platform build (all static)
build-all:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-linux-amd64 ./cmd/ccusage
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-darwin-amd64 ./cmd/ccusage
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-darwin-arm64 ./cmd/ccusage
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-windows-amd64.exe ./cmd/ccusage

# Release builds for different platforms
release-linux:
	@echo "Building Linux releases..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-linux-amd64 ./cmd/ccusage
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-linux-arm64 ./cmd/ccusage
	GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-linux-386 ./cmd/ccusage
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-linux-armv7 ./cmd/ccusage

release-darwin:
	@echo "Building macOS releases..."
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-darwin-amd64 ./cmd/ccusage
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-darwin-arm64 ./cmd/ccusage

release-windows:
	@echo "Building Windows releases..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-windows-amd64.exe ./cmd/ccusage
	GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-windows-386.exe ./cmd/ccusage
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ccusage_go-windows-arm64.exe ./cmd/ccusage

# Build all release targets
release-all: release-linux release-darwin release-windows
	@echo "All release builds completed!"

# Compress release binaries
compress-releases:
	@echo "Compressing release binaries..."
	@cd bin && for file in ccusage_go-linux-* ccusage_go-darwin-*; do \
		if [ -f $$file ]; then \
			tar -czf $$file.tar.gz $$file; \
			echo "Created $$file.tar.gz"; \
		fi \
	done
	@cd bin && for file in ccusage_go-windows-*.exe; do \
		if [ -f $$file ]; then \
			zip $$file.zip $$file; \
			echo "Created $$file.zip"; \
		fi \
	done

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
	go build -gcflags="all=-N -l" -o bin/ccusage_go-dev ./cmd/ccusage

# Dynamic build (non-static, smaller size)
dynamic:
	go build -ldflags="-s -w" -o bin/ccusage_go-dynamic ./cmd/ccusage