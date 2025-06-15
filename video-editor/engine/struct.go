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
	Enabled       bool    `json:"enabled"`
	Color         string  `json:"color"`
	Similarity    float64 `json:"similarity"`
	Blend         float64 `json:"blend"`
	YUVThreshold  float64 `json:"yuv_threshold"`
	EdgeFeather   float64 `json:"edge_feather"`
	AutoAdjust    bool    `json:"auto_adjust"`
	SpillSuppress bool    `json:"spill_suppress"`
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
