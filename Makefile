.PHONY: build run test clean install lint fmt

# Build the application
build:
	go build -o lazystack ./cmd/lazystack

# Run the application
run:
	go run ./cmd/lazystack

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -cover ./...

# Clean build artifacts
clean:
	rm -f lazystack
	go clean

# Install to /usr/local/bin (requires sudo)
install: build
	sudo mv lazystack /usr/local/bin/

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	gofmt -w .

# Vet code
vet:
	go vet ./...

# Tidy modules
tidy:
	go mod tidy

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o dist/lazystack-linux-amd64 ./cmd/lazystack
	GOOS=linux GOARCH=arm64 go build -o dist/lazystack-linux-arm64 ./cmd/lazystack
	GOOS=darwin GOARCH=amd64 go build -o dist/lazystack-darwin-amd64 ./cmd/lazystack
	GOOS=darwin GOARCH=arm64 go build -o dist/lazystack-darwin-arm64 ./cmd/lazystack

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application"
	@echo "  run           - Run the application"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  clean         - Clean build artifacts"
	@echo "  install       - Install to /usr/local/bin"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  tidy          - Tidy modules"
	@echo "  build-all     - Build for multiple platforms"
