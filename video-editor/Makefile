# Go Automated Video Editor Makefile

.PHONY: build run clean setup check-deps test install-deps

# Variables
BINARY_NAME=video-editor
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -type f)

# Default target
all: check-deps build

# Build the application
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) main.go
	@echo "✅ Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run: check-deps
	@echo "🚀 Running video editor..."
	go run main.go

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf output/*
	@echo "✅ Clean completed"

# Setup project directories
setup:
	@echo "📁 Setting up project directories..."
	mkdir -p video-input/{images,audio,overlays,config}
	mkdir -p output
	mkdir -p {models,engine,utils}
	@echo "✅ Project structure created"

# Check if required dependencies are installed
check-deps:
	@echo "🔍 Checking dependencies..."
	@command -v ffmpeg >/dev/null 2>&1 || { echo "❌ FFmpeg is not installed. Please install FFmpeg first."; exit 1; }
	@command -v ffprobe >/dev/null 2>&1 || { echo "❌ FFprobe is not installed. Please install FFmpeg first."; exit 1; }
	@command -v go >/dev/null 2>&1 || { echo "❌ Go is not installed. Please install Go first."; exit 1; }
	@echo "✅ All dependencies are installed"

# Test the application
test:
	@echo "🧪 Running tests..."
	go test ./...

# Install system dependencies (macOS with Homebrew)
install-deps-mac:
	@echo "📦 Installing dependencies on macOS..."
	@command -v brew >/dev/null 2>&1 || { echo "❌ Homebrew is not installed. Please install Homebrew first."; exit 1; }
	brew install ffmpeg
	@echo "✅ Dependencies installed"

# Install system dependencies (Ubuntu/Debian)
install-deps-ubuntu:
	@echo "📦 Installing dependencies on Ubuntu/Debian..."
	sudo apt update
	sudo apt install -y ffmpeg
	@echo "✅ Dependencies installed"

# Create sample project structure with example files
sample:
	@echo "📋 Creating sample project..."
	@$(MAKE) setup
	@echo "Creating sample configuration..."
	@cp project.json video-input/config/ 2>/dev/null || true
	@echo "✅ Sample project created"
	@echo "👉 Add your images to video-input/images/"
	@echo "👉 Add your voice files to video-input/audio/"
	@echo "👉 Add background.mp3 to video-input/audio/"
	@echo "👉 Add overlay files to video-input/overlays/"

# Build for multiple platforms
build-all:
	@echo "🌍 Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)

	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 main.go

	# Windows AMD64
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe main.go

	# macOS AMD64
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 main.go

	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 main.go

	@echo "✅ Multi-platform build completed"

# Development mode with file watching (requires air)
dev:
	@echo "🔄 Starting development mode..."
	@command -v air >/dev/null 2>&1 || { echo "Installing air..."; go install github.com/cosmtrek/air@latest; }
	air

# Format code
fmt:
	@echo "🎨 Formatting code..."
	go fmt ./...
	@echo "✅ Code formatted"

# Lint code (requires golangci-lint)
lint:
	@echo "🔍 Linting code..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; }
	golangci-lint run
	@echo "✅ Code linted"

# Show help
help:
	@echo "🎬 Go Automated Video Editor - Available Commands:"
	@echo ""
	@echo "  build          - Build the application"
	@echo "  run            - Run the application"
	@echo "  clean          - Clean build artifacts and output"
	@echo "  setup          - Create project directory structure"
	@echo "  check-deps     - Check if required dependencies are installed"
	@echo "  test           - Run tests"
	@echo "  sample         - Create sample project with example structure"
	@echo "  build-all      - Build for multiple platforms"
	@echo "  dev            - Start development mode with file watching"
	@echo "  fmt            - Format Go code"
	@echo "  lint           - Lint Go code"
	@echo "  help           - Show this help message"
	@echo ""
	@echo "Platform-specific dependency installation:"
	@echo "  install-deps-mac     - Install dependencies on macOS (Homebrew)"
	@echo "  install-deps-ubuntu  - Install dependencies on Ubuntu/Debian"