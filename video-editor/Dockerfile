# Multi-stage Dockerfile for Go Automated Video Editor

# Build stage
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o video-editor main.go

# Runtime stage
FROM alpine:latest

# Install FFmpeg and other runtime dependencies
RUN apk add --no-cache \
    ffmpeg \
    ffmpeg-dev \
    ca-certificates \
    tzdata

# Create non-root user
RUN addgroup -g 1000 -S videoeditor && \
    adduser -u 1000 -S videoeditor -G videoeditor

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/video-editor .

# Create necessary directories
RUN mkdir -p video-input/{images,audio,overlays,config} output && \
    chown -R videoeditor:videoeditor /app

# Switch to non-root user
USER videoeditor

# Expose volume for input/output
VOLUME ["/app/video-input", "/app/output"]

# Set environment variables
ENV GO_ENV=production

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ffmpeg -version > /dev/null 2>&1 || exit 1

# Default command
CMD ["./video-editor"]