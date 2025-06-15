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
	MaxWorkers int
	WorkerPool chan struct{}
	UseGPU     bool
	GPUDevice  string
	// NEW: Multi-GPU support
	UseMultiGPU bool
	GPUDevices  []GPUDevice    // List of available GPU devices
	GPUPool     chan GPUDevice // Pool of available GPUs for work distribution
}

// NEW: GPU Device structure
type GPUDevice struct {
	Type     string   // "nvidia", "intel", "amd"
	Device   string   // Device identifier
	Encoder  string   // Encoder name (h264_nvenc, h264_qsv, etc.)
	Args     []string // Encoder-specific arguments
	Priority int      // Higher number = higher priority
}
