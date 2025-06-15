package engine

import "youtube_automation/video-editor/models"

// ZoomEffect represents different zoom animation types
type ZoomEffect int

const (
	ZoomIn ZoomEffect = iota
	ZoomOut
	ZoomInOut
	PanZoom
)

// ZoomConfig holds configuration for zoom animations
type ZoomConfig struct {
	Effect     ZoomEffect
	StartScale float64 // Starting zoom scale (1.0 = normal)
	EndScale   float64 // Ending zoom scale
	StartX     float64 // Starting X position (0.5 = center)
	StartY     float64 // Starting Y position (0.5 = center)
	EndX       float64 // Ending X position
	EndY       float64 // Ending Y position
}

// ChromaKeyConfig holds configuration for chroma key removal
type ChromaKeyConfig struct {
	Enabled       bool    // Whether to apply chroma key removal
	Color         string  // Color to remove (e.g., "green", "blue", "#00FF00")
	Similarity    float64 // Color similarity threshold (0.0-1.0, default: 0.3)
	Blend         float64 // Edge blending amount (0.0-1.0, default: 0.1)
	YUVThreshold  float64 // YUV threshold for better keying (0.0-1.0, default: 0.0)
	AutoAdjust    bool    // Auto-adjust thresholds for better results
	SpillSuppress bool    // Enable spill suppression for better edge quality
}

// Add these fields to VideoEditor struct
type VideoEditor struct {
	InputDir   string
	OutputDir  string
	Config     *models.VideoConfig
	MaxWorkers int           // Add this field for configurable concurrency
	WorkerPool chan struct{} // Add this field for worker pool
	UseGPU     bool          // NEW: GPU rendering flag
	GPUDevice  string        // NEW: GPU device (e.g., "0" for first GPU, "cuda", "opencl")
}
