# Multi-stage build for Go application
FROM golang:1.22-alpine AS builder

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
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o whisper-api .

# Final stage - use whisper.cpp base image
FROM ghcr.io/ggml-org/whisper.cpp:main

# Install additional dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    file \
    && rm -rf /var/lib/apt/lists/*

# Create app directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/whisper-api .

# Create necessary directories
RUN mkdir -p /tmp/uploads /models

# Set permissions
RUN chmod +x whisper-api

# Add a script to verify model files
RUN echo '#!/bin/bash\n\
echo "🔍 Checking model files..."\n\
if [ ! -f "/models/ggml-base.bin" ]; then\n\
    echo "❌ Model file not found: /models/ggml-base.bin"\n\
    exit 1\n\
fi\n\
\n\
file_size=$(stat -c%s "/models/ggml-base.bin" 2>/dev/null || echo "0")\n\
if [ "$file_size" -lt 1000000 ]; then\n\
    echo "❌ Model file is too small or corrupted: ${file_size} bytes"\n\
    exit 1\n\
fi\n\
\n\
echo "✅ Model file looks good: ${file_size} bytes"\n\
\n\
# Test whisper binary\n\
if ! command -v whisper-cli > /dev/null 2>&1; then\n\
    echo "❌ whisper-cli binary not found"\n\
    exit 1\n\
fi\n\
\n\
echo "✅ whisper-cli binary found"\n\
echo "🚀 Starting whisper API..."\n\
exec "$@"' > /entrypoint.sh && chmod +x /entrypoint.sh

# Expose port
EXPOSE 8086

# Environment variables
ENV PORT=8086
ENV GIN_MODE=release

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8086/health || exit 1

# Use custom entrypoint
ENTRYPOINT ["/entrypoint.sh"]

# Run the application
CMD ["./whisper-api"]