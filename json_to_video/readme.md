# JSON to Video API - Complete Documentation

## 📋 Table of Contents
1. [Overview](#overview)
2. [Installation & Setup](#installation--setup)
3. [API Flow](#api-flow)
4. [API Endpoints](#api-endpoints)
5. [Usage Examples](#usage-examples)
6. [JSON Schema](#json-schema)
7. [Error Handling](#error-handling)
8. [Best Practices](#best-practices)
9. [Troubleshooting](#troubleshooting)

## 🎯 Overview

The JSON to Video API allows you to generate videos programmatically by sending JSON configurations that define:
- Video dimensions, duration, and background
- Images with positioning and timing
- Audio tracks (background music and voice-over)
- Subtitles from SRT files
- Text overlays with animations and effects

## 🚀 Installation & Setup

### Prerequisites
1. **Go 1.19+** installed
2. **FFmpeg** installed and available in PATH
3. **Git** for cloning dependencies

### Step 1: Setup Project
```bash
# Create project directory
mkdir json-video-api
cd json-video-api

# Initialize Go module
go mod init json-video-api

# Install dependencies
go get github.com/gorilla/mux
go get github.com/google/uuid
```

### Step 2: Install FFmpeg
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install ffmpeg

# macOS
brew install ffmpeg

# Windows
# Download from https://ffmpeg.org/download.html
```

### Step 3: Run the Server
```bash
# Copy the Go code to main.go
go run main.go
```

The server will start on `http://localhost:8080`

## 🔄 API Flow

```mermaid
graph TD
    A[Client] -->|1. Upload Assets| B[Upload Endpoints]
    B -->|2. Get URLs| A
    A -->|3. Submit JSON| C[/api/generate]
    C -->|4. Create Job| D[Job Queue]
    D -->|5. Return Job ID| A
    A -->|6. Poll Status| E[/api/status/{jobId}]
    E -->|7. Return Progress| A
    D -->|8. Process Video| F[FFmpeg Engine]
    F -->|9. Generate Video| G[Output Storage]
    E -->|10. Video Ready| A
    A -->|11. Download Video| H[/videos/{filename}]
```

### Typical Workflow:
1. **Upload Assets** (optional) - Upload images, audio, or subtitle files
2. **Prepare JSON** - Create video configuration with asset references
3. **Submit Request** - POST to `/api/generate` with JSON payload
4. **Get Job ID** - Receive unique job identifier
5. **Poll Status** - Check progress using `/api/status/{jobId}`
6. **Download Video** - Get completed video from `/videos/{filename}`

## 📡 API Endpoints

### 1. Video Generation
```http
POST /api/generate
Content-Type: application/json

{
  "title": "My Video",
  "duration": 30,
  "width": 1920,
  "height": 1080,
  // ... full JSON configuration
}
```

**Response:**
```json
{
  "job_id": "uuid-here",
  "status": "pending",
  "message": "Video generation started",
  "progress": 0
}
```

### 2. Job Status
```http
GET /api/status/{jobId}
```

**Response:**
```json
{
  "job_id": "uuid-here",
  "status": "completed",
  "message": "Video generation completed successfully",
  "progress": 100,
  "video_url": "/videos/My_Video_12345678.mp4"
}
```

### 3. File Uploads
```http
POST /api/upload/image
Content-Type: multipart/form-data

file: [binary image data]
```

**Response:**
```json
{
  "filename": "uuid_image.png",
  "url": "/assets/images/uuid_image.png",
  "type": "images"
}
```

### 4. List All Jobs
```http
GET /api/jobs
```

### 5. Cancel Job
```http
DELETE /api/cancel/{jobId}
```

### 6. Health Check
```http
GET /health
```

## 💡 Usage Examples

### Example 1: Simple Text Video
```bash
curl -X POST http://localhost:8080/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Hello World",
    "duration": 10,
    "width": 1280,
    "height": 720,
    "background": "#0066cc",
    "scenes": [
      {
        "start_time": 0,
        "duration": 5,
        "text": "Hello, World!",
        "font_size": 48,
        "font_color": "white",
        "position": "center",
        "effect": "fade"
      },
      {
        "start_time": 5,
        "duration": 5,
        "text": "Welcome to JSON Video API",
        "font_size": 32,
        "font_color": "yellow",
        "position": "bottom"
      }
    ]
  }'
```

### Example 2: Video with Images and Audio
```bash
# First upload an image
curl -X POST http://localhost:8080/api/upload/image \
  -F "file=@logo.png"

# Then create video with the uploaded image
curl -X POST http://localhost:8080/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Brand Video",
    "duration": 20,
    "width": 1920,
    "height": 1080,
    "background": "#1a1a1a",
    "images": [
      {
        "id": "logo",
        "url": "/assets/images/uuid_logo.png",
        "start_time": 0,
        "duration": 20,
        "x": 100,
        "y": 100,
        "width": 300,
        "height": 150,
        "opacity": 0.9
      }
    ],
    "audio": {
      "background_url": "/assets/audio/background.mp3",
      "volume": 0.3
    },
    "scenes": [
      {
        "start_time": 2,
        "duration": 8,
        "text": "Your Brand Here",
        "font_size": 64,
        "font_color": "white",
        "position": "center"
      }
    ]
  }'
```

### Example 3: Video with Subtitles
```bash
curl -X POST http://localhost:8080/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Subtitle Demo",
    "duration": 15,
    "width": 1280,
    "height": 720,
    "background": "#2c3e50",
    "subtitles": {
      "srt_data": "1\n00:00:00,000 --> 00:00:05,000\nWelcome to our presentation\n\n2\n00:00:05,000 --> 00:00:10,000\nThis video was generated from JSON\n\n3\n00:00:10,000 --> 00:00:15,000\nThank you for watching!",
      "font_size": 24,
      "font_color": "white",
      "position": "bottom",
      "outline": true
    }
  }'
```

### Example 4: JavaScript Client Integration
```javascript
class VideoAPI {
  constructor(baseUrl = 'http://localhost:8080') {
    this.baseUrl = baseUrl;
  }

  async generateVideo(config) {
    const response = await fetch(`${this.baseUrl}/api/generate`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(config)
    });
    
    return await response.json();
  }

  async checkStatus(jobId) {
    const response = await fetch(`${this.baseUrl}/api/status/${jobId}`);
    return await response.json();
  }

  async uploadFile(file, type) {
    const formData = new FormData();
    formData.append('file', file);
    
    const response = await fetch(`${this.baseUrl}/api/upload/${type}`, {
      method: 'POST',
      body: formData
    });
    
    return await response.json();
  }

  async waitForCompletion(jobId, onProgress = null) {
    return new Promise((resolve, reject) => {
      const checkInterval = setInterval(async () => {
        try {
          const status = await this.checkStatus(jobId);
          
          if (onProgress) {
            onProgress(status.progress, status.status);
          }
          
          if (status.status === 'completed') {
            clearInterval(checkInterval);
            resolve(status);
          } else if (status.status === 'failed') {
            clearInterval(checkInterval);
            reject(new Error(status.message));
          }
        } catch (error) {
          clearInterval(checkInterval);
          reject(error);
        }
      }, 2000); // Check every 2 seconds
    });
  }
}

// Usage example
const api = new VideoAPI();

async function createVideo() {
  try {
    // Generate video
    const job = await api.generateVideo({
      title: "My Video",
      duration: 10,
      width: 1280,
      height: 720,
      background: "#3498db",
      scenes: [{
        start_time: 0,
        duration: 10,
        text: "Hello from JavaScript!",
        font_size: 48,
        font_color: "white",
        position: "center"
      }]
    });

    console.log('Job started:', job.job_id);

    // Wait for completion
    const result = await api.waitForCompletion(job.job_id, (progress, status) => {
      console.log(`Progress: ${progress}% - ${status}`);
    });

    console.log('Video ready:', result.video_url);
  } catch (error) {
    console.error('Error:', error);
  }
}
```

## 📝 JSON Schema

### Root Configuration
```json
{
  "title": "string (required)",
  "duration": "number (required, seconds)",
  "width": "number (required, pixels)",
  "height": "number (required, pixels)",
  "background": "string (hex color or image path)",
  "images": "array of ImageAsset",
  "audio": "AudioConfig object",
  "subtitles": "SubtitleConfig object",
  "scenes": "array of Scene objects"
}
```

### ImageAsset
```json
{
  "id": "string (unique identifier)",
  "data": "string (base64 encoded image)",
  "url": "string (image URL or path)",
  "start_time": "number (seconds)",
  "duration": "number (seconds)",
  "x": "number (pixels from left)",
  "y": "number (pixels from top)",
  "width": "number (scaled width)",
  "height": "number (scaled height)",
  "z_index": "number (layer order)",
  "opacity": "number (0.0 to 1.0)",
  "effect": "string (fade, slide, zoom, none)"
}
```

### AudioConfig
```json
{
  "background_music": "string (base64 encoded)",
  "background_url": "string (audio file URL)",
  "volume": "number (0.0 to 1.0)",
  "fade_in": "number (seconds)",
  "fade_out": "number (seconds)",
  "voice_over": "string (base64 encoded)",
  "voice_over_url": "string (audio file URL)",
  "voice_volume": "number (0.0 to 1.0)"
}
```

### SubtitleConfig
```json
{
  "srt_data": "string (SRT format content)",
  "srt_url": "string (SRT file URL)",
  "font_size": "number (pixels)",
  "font_color": "string (color name or hex)",
  "position": "string (bottom, top, center)",
  "background": "string (background color)",
  "outline": "boolean (text outline)"
}
```

### Scene
```json
{
  "start_time": "number (seconds)",
  "duration": "number (seconds)",
  "text": "string (text content)",
  "font_size": "number (pixels)",
  "font_color": "string (color)",
  "position": "string (center, top, bottom)",
  "x": "number (custom x position)",
  "y": "number (custom y position)",
  "effect": "string (fade, slide, typewriter, none)",
  "background": "string (text background color)",
  "outline": "boolean (text outline)",
  "animation": "string (bounce, shake, pulse)"
}
```

## ⚠️ Error Handling

### Common Error Responses
```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": "Additional error details"
}
```

### HTTP Status Codes
- `200` - Success
- `400` - Bad Request (invalid JSON, missing required fields)
- `404` - Not Found (job not found)
- `500` - Internal Server Error (FFmpeg error, file system error)

### Job Status Values
- `pending` - Job queued for processing
- `processing` - Video generation in progress
- `completed` - Video generated successfully
- `failed` - Video generation failed
- `cancelled` - Job cancelled by user

## 🎯 Best Practices

### 1. Asset Management
- **Upload files first** using upload endpoints
- **Use reasonable file sizes** (< 10MB per file)
- **Validate file formats** before uploading
- **Clean up unused assets** periodically

### 2. Video Configuration
- **Keep duration reasonable** (< 5 minutes for better performance)
- **Use standard resolutions** (720p, 1080p, 4K)
- **Optimize scene timing** to avoid overlaps
- **Test with simple configs** before complex ones

### 3. Performance Optimization
- **Poll status reasonably** (every 2-5 seconds)
- **Handle timeouts gracefully**
- **Use appropriate video dimensions**
- **Compress assets when possible**

### 4. Error Handling
```javascript
async function robustVideoGeneration(config) {
  const maxRetries = 3;
  let retries = 0;
  
  while (retries < maxRetries) {
    try {
      const job = await api.generateVideo(config);
      const result = await api.waitForCompletion(job.job_id);
      return result;
    } catch (error) {
      retries++;
      if (retries >= maxRetries) {
        throw error;
      }
      await new Promise(resolve => setTimeout(resolve, 1000 * retries));
    }
  }
}
```

## 🔧 Troubleshooting

### Common Issues

#### 1. FFmpeg Not Found
```bash
# Check if FFmpeg is installed
ffmpeg -version

# Install if missing (Ubuntu)
sudo apt install ffmpeg
```

#### 2. File Upload Fails
- Check file size limits (10MB default)
- Verify file format is supported
- Ensure sufficient disk space

#### 3. Video Generation Fails
- Check FFmpeg logs in server output
- Validate JSON schema
- Verify asset file paths
- Check video duration and dimensions

#### 4. Slow Performance
- Reduce video resolution
- Optimize asset file sizes
- Limit concurrent jobs
- Use SSD storage for better I/O

### Debug Mode
```bash
# Run with verbose logging
DEBUG=1 go run main.go
```

### Health Check
```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T00:00:00Z",
  "version": "2.0.0",
  "ffmpeg_available": true,
  "active_jobs": 0
}
```

## 📚 Advanced Examples

### Python Client
```python
import requests
import time
import json

class VideoAPIClient:
    def __init__(self, base_url="http://localhost:8080"):
        self.base_url = base_url
    
    def generate_video(self, config):
        response = requests.post(f"{self.base_url}/api/generate", json=config)
        response.raise_for_status()
        return response.json()
    
    def check_status(self, job_id):
        response = requests.get(f"{self.base_url}/api/status/{job_id}")
        response.raise_for_status()
        return response.json()
    
    def wait_for_completion(self, job_id):
        while True:
            status = self.check_status(job_id)
            print(f"Progress: {status['progress']}% - {status['status']}")
            
            if status['status'] == 'completed':
                return status
            elif status['status'] == 'failed':
                raise Exception(status['message'])
            
            time.sleep(2)

# Usage
client = VideoAPIClient()

config = {
    "title": "Python Generated Video",
    "duration": 15,
    "width": 1280,
    "height": 720,
    "background": "#e74c3c",
    "scenes": [
        {
            "start_time": 0,
            "duration": 15,
            "text": "Generated with Python!",
            "font_size": 48,
            "font_color": "white",
            "position": "center"
        }
    ]
}

job = client.generate_video(config)
result = client.wait_for_completion(job['job_id'])
print(f"Video ready: {result['video_url']}")
```

## 🔑 API Keys & Security

For production use, consider adding:
- **API key authentication**
- **Rate limiting**
- **File upload restrictions**
- **CORS configuration**
- **HTTPS enforcement**

Example security middleware:
```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        apiKey := r.Header.Get("X-API-Key")
        if apiKey != "your-secret-key" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

---

## 📞 Support

For issues and questions:
1. Check server logs for errors
2. Verify FFmpeg installation
3. Test with minimal JSON configurations
4. Check file permissions and disk space

This API provides a powerful foundation for programmatic video generation. Start with simple examples and gradually build more complex video compositions!