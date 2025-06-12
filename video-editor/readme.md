# 🎬 Go Automated Video Editor

A powerful Go-based automated video editor that uses FFmpeg to create professional videos from structured input folders. This tool merges voiceovers, background music, slideshows, and animated overlays with keyframe-based animations.

## ✨ Features

- 🎙️ **Automatic Voice Merging**: Concatenates multiple voice files in order
- 🔊 **Smart Background Music**: Loops and extends background music to match video duration
- 🖼️ **Dynamic Slideshow**: Creates smooth transitions between images
- 🎛️ **Keyframe Animations**: Supports position, scale, and opacity animations for overlays and text
- 📐 **Flexible Configuration**: JSON-based project configuration
- 🚀 **High Performance**: Leverages FFmpeg's powerful video processing capabilities

## 🛠️ Prerequisites

Before running the video editor, ensure you have:

1. **Go 1.21+** installed
2. **FFmpeg** and **FFprobe** installed and available in your PATH
3. Proper input folder structure (see below)

### Installing FFmpeg

**Windows:**
```bash
# Using Chocolatey
choco install ffmpeg

# Or download from https://ffmpeg.org/download.html
```

**macOS:**
```bash
# Using Homebrew
brew install ffmpeg
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update
sudo apt install ffmpeg
```

## 📁 Project Structure

```
video-editor/
├── main.go
├── go.mod
├── models/
│   └── config.go
├── engine/
│   └── video_editor.go
├── utils/
│   └── file_utils.go
├── video-input/
│   ├── images/
│   │   ├── image01.jpg
│   │   ├── image02.jpg
│   │   └── imageN.jpg
│   ├── audio/
│   │   ├── background.mp3
│   │   ├── voice01.mp3
│   │   ├── voice02.mp3
│   │   └── voiceN.mp3
│   ├── overlays/
│   │   ├── logo.png
│   │   └── sticker.gif
│   └── config/
│       └── project.json
└── output/
    ├── merged_voice.mp3
    ├── extended_bgm.mp3
    ├── slideshow.mp4
    └── final_video.mp4
```

## 🚀 Quick Start

1. **Clone or create the project structure:**
```bash
mkdir video-editor
cd video-editor
```

2. **Initialize Go module:**
```bash
go mod init video-editor
```

3. **Create the required directories:**
```bash
mkdir -p video-input/{images,audio,overlays,config}
mkdir -p output
mkdir -p {models,engine,utils}
```

4. **Add your content:**
    - Place images in `video-input/images/`
    - Add voice files (`voice01.mp3`, `voice02.mp3`, etc.) in `video-input/audio/`
    - Add `background.mp3` in `video-input/audio/`
    - Place overlay images/GIFs in `video-input/overlays/`
    - Create `project.json` configuration in `video-input/config/`

5. **Run the video editor:**
```bash
go run main.go
```

## ⚙️ Configuration

The `project.json` file controls all aspects of your video. Here's a comprehensive example:

### Settings Section
```json
{
  "settings": {
    "width": 1920,        // Video width in pixels
    "height": 1080,       // Video height in pixels  
    "fps": 30,            // Frames per second
    "bgm_volume": 0.3,    // Background music volume (0.0 - 1.0)
    "voice_volume": 1.0   // Voice volume (0.0 - 1.0)
  }
}
```

### Overlays Section
```json
{
  "overlays": [
    {
      "source": "logo.png",    // File from overlays/ folder
      "start": 2.0,            // Start time in seconds
      "end": 7.0,              // End time in seconds
      "keyframes": [
        {
          "time": 2.0,         // Keyframe time
          "x": 1280,           // X position
          "y": 720,            // Y position  
          "opacity": 0.0,      // Opacity (0.0 - 1.0)
          "scale": 0.5         // Scale factor
        }
      ]
    }
  ]
}
```

### Text Section
```json
{
  "texts": [
    {
      "text": "Welcome to My Video!",
      "start": 5.0,
      "end": 10.0,
      "font_size": 48,
      "font_color": "white",
      "keyframes": [
        {
          "time": 5.0,
          "x": 640,
          "y": 100,
          "opacity": 0.0,
          "scale": 1.0
        }
      ]
    }
  ]
}
```

## 🎨 Animation System

The keyframe animation system supports smooth interpolation between keyframes:

### Supported Properties
- **Position**: `x`, `y` coordinates
- **Opacity**: `opacity` (0.0 = transparent, 1.0 = opaque)
- **Scale**: `scale` (1.0 = original size, 2.0 = double size)

### Animation Types
- **Linear Interpolation**: Smooth transitions between keyframes
- **Multiple Keyframes**: Create complex animation sequences
- **Time-based Control**: Precise timing control

### Example Animation Sequence
```json
{
  "keyframes": [
    {"time": 0, "x": 0, "y": 0, "opacity": 0},      // Start invisible at top-left
    {"time": 1, "x": 500, "y": 300, "opacity": 1},   // Fade in while moving
    {"time": 3, "x": 1000, "y": 300, "opacity": 1},  // Continue moving
    {"time": 4, "x": 1200, "y": 400, "opacity": 0}   // Fade out while moving
  ]
}
```

## 📊 Processing Pipeline

The video editor follows this processing pipeline:

1. **🎙️ Voice Merging**: Concatenates all `voice*.mp3` files in alphabetical order
2. **📏 Duration Calculation**: Uses FFprobe to measure total voice duration
3. **🔊 Background Music Extension**: Loops background music to match voice duration
4. **🖼️ Slideshow Creation**: Creates video slideshow with fade transitions
5. **🎛️ Effects Application**: Applies overlays and text with keyframe animations
6. **🎵 Audio Mixing**: Combines voice and background music with volume control
7. **📹 Final Rendering**: Outputs the complete video

## 🔧 Advanced Usage

### Custom FFmpeg Parameters

You can modify the FFmpeg parameters in `engine/video_editor.go`:

```go
// For higher quality output
args = append(args, "-preset", "slow", "-crf", "18")

// For faster encoding  
args = append(args, "-preset", "ultrafast", "-crf", "28")

// For different codecs
args = append(args, "-c:v", "libx265") // H.265 encoding
```

### Supported File Formats

**Images**: JPG, JPEG, PNG, BMP, TIFF  
**Audio**: MP3, WAV, M4A  
**Overlays**: PNG (with transparency), GIF (animated)

### Performance Optimization

- Use lower resolution images for faster processing
- Reduce keyframe complexity for better performance
- Use appropriate CRF values (18-28) for quality vs. size balance

## 🐛 Troubleshooting

### Common Issues

**FFmpeg not found:**
```
Error: ffmpeg not found in PATH
Solution: Install FFmpeg and ensure it's in your system PATH
```

**No voice files found:**
```
Error: no voice files found
Solution: Ensure voice files are named voice01.mp3, voice02.mp3, etc.
```

**Config parsing error:**
```
Error: failed to load config
Solution: Validate your project.json syntax using a JSON validator
```

### Debug Mode

Add debug output by modifying the FFmpeg commands:
```go
cmd := exec.Command("ffmpeg", append([]string{"-v", "verbose"}, args...)...)
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues, feature requests, or pull requests.

### Development Setup

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📝 License

This project is open source. Feel free to use and modify as needed.

## 🙏 Acknowledgments

- FFmpeg team for the powerful multimedia framework
- Go community for excellent tooling and libraries

---

**Happy Video Editing! 🎬✨**