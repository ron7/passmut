.PHONY: build build-dev install clean help fmt vet run version test all

# Binary name
BINARY_NAME=passmut

# Build the binary (production - optimized and stripped)
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME) main.go

# Build with optimizations for production
build-dev:
	go build -o $(BINARY_NAME) main.go

# Install the binary to GOPATH/bin
install:
	go install

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f test_output.txt
	rm -f *.exe

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Run the tool with example input
run: build-dev
	@echo "test\nword" | ./$(BINARY_NAME)

# Show version
version: build-dev
	./$(BINARY_NAME) -v

# Show help
help: build-dev
	./$(BINARY_NAME) -h

# Run all checks (fmt, vet)
all: fmt vet build

# Cross-compilation targets
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_NAME)-linux-amd64 main.go

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_NAME)-windows-amd64.exe main.go

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_NAME)-darwin-amd64 main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(BINARY_NAME)-darwin-arm64 main.go

# Build for all platforms
build-all: build-linux build-windows build-darwin

# Test with analyze mode
test-analyze: build-dev
	@echo -e "password\nadmin\ntest123" | ./$(BINARY_NAME) --analyze

# Test basic mutations
test-basic: build-dev
	@echo -e "test\nword" | ./$(BINARY_NAME) --capital --leet
