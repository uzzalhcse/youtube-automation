package engine

import (
	"log"
	"os/exec"
	"runtime"
	"strings"
)

func (ve *VideoEditor) getEncoderSettings() (string, []string) {
	if ve.UseGPU {
		// Check if a specific GPU type is requested in config
		requestedGPU := ve.getRequestedGPUType()
		if requestedGPU != "" {
			log.Printf("🎯 Requested GPU type: %s", requestedGPU)
			if ve.isGPUTypeAvailable(requestedGPU) {
				return ve.getEncoderForGPUType(requestedGPU)
			} else {
				log.Printf("⚠️ Requested GPU type %s not available, using auto-detection", requestedGPU)
			}
		}

		// Auto-detect GPU type with priority
		gpuType := ve.detectGPUType()
		log.Printf("🔍 Auto-detected GPU type: %s", gpuType)

		return ve.getEncoderForGPUType(gpuType)
	}

	// CPU fallback - optimized for different environments
	return ve.getCPUEncoderSettings()
}

// NEW: Check if specific GPU type is available
func (ve *VideoEditor) isGPUTypeAvailable(gpuType string) bool {
	switch gpuType {
	case "nvidia":
		return ve.isNVIDIAGPUAvailable()
	case "amd":
		return ve.isAMDGPUAvailable()
	case "intel":
		return ve.isIntelGPUAvailable()
	default:
		return false
	}
}

// NEW: Get encoder for specific GPU type
func (ve *VideoEditor) getEncoderForGPUType(gpuType string) (string, []string) {
	switch gpuType {
	case "nvidia":
		return ve.getNVIDIAEncoderSettings()
	case "amd":
		return ve.getAMDEncoderSettings()
	case "intel":
		return ve.getIntelEncoderSettings()
	default:
		log.Printf("⚠️ Unknown GPU type '%s', falling back to CPU encoding", gpuType)
		return ve.getCPUEncoderSettings()
	}
}

// NEW: Get all available GPUs
func (ve *VideoEditor) getAvailableGPUs() []string {
	var available []string

	// Check NVIDIA
	if ve.isNVIDIAGPUAvailable() {
		available = append(available, "nvidia")
	}

	// Check AMD
	if ve.isAMDGPUAvailable() {
		available = append(available, "amd")
	}

	// Check Intel
	if ve.isIntelGPUAvailable() {
		available = append(available, "intel")
	}

	return available
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (ve *VideoEditor) isNVIDIAGPUAvailable() bool {
	log.Printf("🔍 Checking NVIDIA GPU availability...")

	// Method 1: Check nvidia-smi for GPU presence
	// Try common NVIDIA paths
	nvidiaPaths := []string{
		"nvidia-smi", // Try PATH first
		//`C:\Windows\System32\DriverStore\FileRepository\nvhmi.inf_amd64_7c0e4faf7f6038f7\nvidia-smi.exe`,
	}

	var cmd *exec.Cmd
	var output []byte
	var err error

	for _, path := range nvidiaPaths {
		cmd = exec.Command(path, "-L")
		output, err = cmd.Output()
		if err == nil {
			break
		}
		log.Printf("Tried path %s: %v", path, err)
	}

	if err != nil {
		log.Printf("nvidia-smi not available in any known location: %v", err)
		return false
	}

	// Log detected NVIDIA GPUs
	gpuList := strings.TrimSpace(string(output))
	if gpuList == "" {
		log.Printf("❌ No NVIDIA GPUs found via nvidia-smi")
		return false
	}

	log.Printf("🎮 Detected NVIDIA GPUs:\n%s", gpuList)

	// Method 2: Check if NVENC encoder is available in FFmpeg with detailed testing
	if !ve.testNVENCEncoder() {
		log.Printf("❌ NVIDIA GPU found but NVENC encoder not working properly")
		return false
	}

	log.Printf("✅ NVIDIA GPU detected and NVENC confirmed working")
	return true
}

// New method: More thorough NVENC encoder testing
func (ve *VideoEditor) testNVENCEncoder() bool {
	log.Printf("🧪 Testing NVENC encoder availability...")

	// First, check if h264_nvenc encoder exists in FFmpeg
	cmd := exec.Command("ffmpeg", "-hide_banner", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("❌ Failed to get FFmpeg encoders list: %v", err)
		return false
	}

	encodersList := string(output)
	if !strings.Contains(encodersList, "h264_nvenc") {
		log.Printf("❌ h264_nvenc encoder not found in FFmpeg build")
		return false
	}

	log.Printf("✅ h264_nvenc encoder found in FFmpeg")

	// Test actual encoding with NVENC - more comprehensive test
	testCmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-t", "1",
		"-c:v", "h264_nvenc",
		"-preset", "fast", // Use compatible preset
		"-f", "null", "-")

	err = testCmd.Run()
	if err != nil {
		log.Printf("❌ NVENC encoder test failed: %v", err)

		// Try with different preset for older cards
		log.Printf("🔄 Retrying NVENC test with legacy preset...")
		testCmd2 := exec.Command("ffmpeg",
			"-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
			"-t", "1",
			"-c:v", "h264_nvenc",
			"-preset", "default", // Legacy preset
			"-f", "null", "-")

		err2 := testCmd2.Run()
		if err2 != nil {
			log.Printf("❌ NVENC encoder test failed with legacy preset: %v", err2)
			return false
		}
	}

	log.Printf("✅ NVENC encoder test successful")
	return true
}

// NEW: AMD GPU detection
func (ve *VideoEditor) isAMDGPUAvailable() bool {
	// Check for AMD GPU tools
	if cmd := exec.Command("rocm-smi"); cmd.Run() == nil {
		return ve.isEncoderAvailable("h264_amf")
	}

	// Alternative check for Windows AMD drivers
	if runtime.GOOS == "windows" {
		return ve.isEncoderAvailable("h264_amf")
	}

	return false
}

// NEW: Intel GPU detection with better logging
func (ve *VideoEditor) isIntelGPUAvailable() bool {
	if ve.isEncoderAvailable("h264_qsv") {
		log.Printf("🔵 Intel QuickSync (h264_qsv) available")
		return true
	}
	return false
}

// NEW: Get AMD-specific encoder settings
func (ve *VideoEditor) getAMDEncoderSettings() (string, []string) {
	if ve.isEncoderAvailable("h264_amf") {
		log.Printf("🔴 Using AMD GPU encoding")
		return "h264_amf", []string{
			"-quality", "speed",
			"-rc", "vbr_peak",
			"-qp_i", "20",
			"-qp_p", "22",
			"-qp_b", "24",
			"-b:v", "5M",
			"-maxrate", "8M",
		}
	}

	log.Printf("⚠️ AMD GPU detected but h264_amf not available, falling back to CPU")
	return ve.getCPUEncoderSettings()
}

// NEW: Get Intel-specific encoder settings
func (ve *VideoEditor) getIntelEncoderSettings() (string, []string) {
	if ve.isEncoderAvailable("h264_qsv") {
		log.Printf("🔵 Using Intel GPU encoding")
		return "h264_qsv", []string{
			"-preset", "fast",
			"-global_quality", "20",
			"-b:v", "4M",
			"-maxrate", "6M",
		}
	}

	log.Printf("⚠️ Intel GPU detected but h264_qsv not available, falling back to CPU")
	return ve.getCPUEncoderSettings()
}

// NEW: Get CPU encoder settings based on environment
func (ve *VideoEditor) getCPUEncoderSettings() (string, []string) {
	log.Printf("🖥️ Using CPU encoding")

	if ve.isGoogleColab() {
		return "libx264", []string{
			"-preset", "fast", // Faster for Colab's limited resources
			"-crf", "23",
			"-threads", "2",
			"-tune", "film",
		}
	}

	// Local PC CPU settings
	return "libx264", []string{
		"-preset", "medium", // Better quality for local processing
		"-crf", "21",
		"-threads", "0", // Use all available CPU threads
	}
}

// Modified isEncoderAvailable method with better error handling
func (ve *VideoEditor) isEncoderAvailable(encoder string) bool {
	// Create a test command to check if the encoder works
	testCmd := exec.Command("ffmpeg", "-hide_banner", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-t", "1", "-c:v", encoder, "-f", "null", "-")

	// Run the test command and check if it succeeds
	err := testCmd.Run()
	if err != nil {
		log.Printf("Encoder %s test failed: %v", encoder, err)
		return false
	}

	log.Printf("✅ Encoder %s is available and working", encoder)
	return true
}

// Updated getRequestedGPUType with better local NVIDIA preference
func (ve *VideoEditor) getRequestedGPUType() string {
	// For local systems, prefer NVIDIA over Intel integrated graphics
	if !ve.isGoogleColab() {
		// Check if we have NVIDIA GPU available
		if ve.checkNVIDIAPresence() {
			log.Printf("🎯 Local system detected, prioritizing NVIDIA GPU")
			return "nvidia"
		}
	}

	return "" // Empty means continue with normal auto-detection
}

// New helper method: Quick NVIDIA presence check (without full testing)
func (ve *VideoEditor) checkNVIDIAPresence() bool {
	cmd := exec.Command("nvidia-smi", "-L")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	gpuList := strings.TrimSpace(string(output))
	return gpuList != ""
}

// Updated detectGPUType with better priority logic
func (ve *VideoEditor) detectGPUType() string {
	// Check what GPUs are available
	availableGPUs := ve.getAvailableGPUs()
	log.Printf("🔍 Available GPUs: %v", availableGPUs)

	// For local systems (non-Colab), prioritize discrete GPUs
	if !ve.isGoogleColab() {
		// Priority order for local: NVIDIA > AMD > Intel
		if contains(availableGPUs, "nvidia") {
			log.Printf("🎯 Local system: Prioritizing NVIDIA discrete GPU")
			return "nvidia"
		}

		if contains(availableGPUs, "amd") {
			log.Printf("🎯 Local system: Using AMD discrete GPU")
			return "amd"
		}

		if contains(availableGPUs, "intel") {
			log.Printf("🎯 Local system: Using Intel integrated GPU (no discrete GPU available)")
			return "intel"
		}
	} else {
		// For Colab, different priority (T4 detection logic)
		if contains(availableGPUs, "nvidia") && ve.isT4Available() {
			log.Printf("🎯 Colab T4 GPU detected")
			return "nvidia"
		}
	}

	return "unknown"
}

// Updated NVIDIA encoder settings with better local PC support
func (ve *VideoEditor) getNVIDIAEncoderSettings() (string, []string) {
	log.Printf("🚀 Configuring NVIDIA GPU encoding...")

	// Check if running in Google Colab with T4
	if ve.isGoogleColab() && ve.isT4Available() {
		log.Printf("🚀 Using T4 GPU settings for Google Colab")
		return "h264_nvenc", []string{
			"-preset", "fast",
			"-tune", "hq",
			"-rc", "vbr",
			"-cq", "19",
			"-b:v", "8M",
			"-maxrate", "12M",
			"-bufsize", "16M",
			"-profile:v", "high",
			"-level", "4.1",
			"-bf", "3",
			"-g", "250",
		}
	}

	// For local PC - detect GPU generation and use appropriate settings
	gpuGeneration := ve.getNVIDIAGPUGeneration()
	log.Printf("🎮 NVIDIA GPU generation: %s", gpuGeneration)

	switch gpuGeneration {
	case "rtx40", "rtx30":
		// Latest RTX cards - use advanced settings
		log.Printf("🚀 Using modern RTX GPU settings (AV1 capable)")
		return "h264_nvenc", []string{
			"-preset", "p4",
			"-tune", "hq",
			"-rc", "vbr",
			"-cq", "20",
			"-b:v", "6M",
			"-maxrate", "10M",
			"-bufsize", "12M",
			"-profile:v", "high",
			"-level", "4.1",
			"-bf", "3",
			"-spatial_aq", "1",
			"-temporal_aq", "1",
		}

	case "rtx20":
		// RTX 20 series - good balance
		log.Printf("🚀 Using RTX 20 series GPU settings")
		return "h264_nvenc", []string{
			"-preset", "slow", // Better quality for RTX 20
			"-tune", "hq",
			"-rc", "vbr",
			"-cq", "21",
			"-b:v", "5M",
			"-maxrate", "8M",
			"-bufsize", "10M",
			"-profile:v", "high",
			"-bf", "3",
			"-spatial_aq", "1",
		}

	case "gtx16", "gtx10":
		// Older GTX cards - use compatible settings
		log.Printf("🎮 Using GTX GPU settings (legacy compatible)")
		return "h264_nvenc", []string{
			"-preset", "medium",
			"-rc", "vbr",
			"-cq", "22",
			"-b:v", "4M",
			"-maxrate", "6M",
			"-bufsize", "8M",
			"-profile:v", "main",
			"-bf", "2",
		}

	default:
		// Generic NVIDIA settings - most compatible
		log.Printf("🔧 Using generic NVIDIA GPU settings (maximum compatibility)")
		return "h264_nvenc", []string{
			"-preset", "default", // Most compatible preset
			"-rc", "cbr", // Constant bitrate for compatibility
			"-b:v", "4M",
			"-profile:v", "baseline", // Most compatible profile
			"-level", "3.1",
		}
	}
}

// Enhanced GPU generation detection
func (ve *VideoEditor) getNVIDIAGPUGeneration() string {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ Could not detect GPU model: %v", err)
		return "unknown"
	}

	gpuName := strings.ToLower(strings.TrimSpace(string(output)))
	log.Printf("🔍 Detected GPU: %s", gpuName)

	// RTX 40 series
	if strings.Contains(gpuName, "rtx 4") || strings.Contains(gpuName, "rtx4") {
		return "rtx40"
	}

	// RTX 30 series
	if strings.Contains(gpuName, "rtx 3") || strings.Contains(gpuName, "rtx30") {
		return "rtx30"
	}

	// RTX 20 series
	if strings.Contains(gpuName, "rtx 2") || strings.Contains(gpuName, "rtx20") ||
		strings.Contains(gpuName, "rtx 2060") || strings.Contains(gpuName, "rtx 2070") ||
		strings.Contains(gpuName, "rtx 2080") {
		return "rtx20"
	}

	// GTX 16 series
	if strings.Contains(gpuName, "gtx 16") || strings.Contains(gpuName, "gtx16") ||
		strings.Contains(gpuName, "gtx 1660") || strings.Contains(gpuName, "gtx 1650") {
		return "gtx16"
	}

	// GTX 10 series
	if strings.Contains(gpuName, "gtx 10") || strings.Contains(gpuName, "gtx10") ||
		strings.Contains(gpuName, "gtx 1080") || strings.Contains(gpuName, "gtx 1070") ||
		strings.Contains(gpuName, "gtx 1060") || strings.Contains(gpuName, "gtx 1050") {
		return "gtx10"
	}

	// Quadro and Tesla cards
	if strings.Contains(gpuName, "quadro") || strings.Contains(gpuName, "tesla") {
		return "workstation"
	}

	return "unknown"
}

// Add debugging method to help troubleshoot
func (ve *VideoEditor) debugGPUSetup() {
	log.Printf("🔧 GPU Setup Debug Information:")
	log.Printf("   - UseGPU: %v", ve.UseGPU)
	log.Printf("   - GPUDevice: %s", ve.GPUDevice)
	log.Printf("   - IsGoogleColab: %v", ve.isGoogleColab())

	// Test nvidia-smi
	cmd := exec.Command("nvidia-smi", "-L")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("   - nvidia-smi: FAILED (%v)", err)
	} else {
		log.Printf("   - nvidia-smi: SUCCESS")
		log.Printf("   - GPUs found: %s", strings.TrimSpace(string(output)))
	}

	// Test FFmpeg encoders
	encoders := []string{"h264_nvenc", "h264_qsv", "h264_amf"}
	for _, encoder := range encoders {
		available := ve.testSpecificEncoder(encoder)
		log.Printf("   - %s: %v", encoder, available)
	}
}

// Helper method to test specific encoder
func (ve *VideoEditor) testSpecificEncoder(encoder string) bool {
	testCmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "panic",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-t", "1",
		"-c:v", encoder,
		"-f", "null", "-")

	return testCmd.Run() == nil
}

// Updated optimizeForColab method with multi-GPU support
func (ve *VideoEditor) optimizeForColab() {
	if ve.isGoogleColab() {
		log.Printf("🔧 Optimizing settings for Google Colab environment")

		if ve.MaxWorkers > 2 {
			ve.MaxWorkers = 2
			ve.WorkerPool = make(chan struct{}, ve.MaxWorkers)
			log.Printf("Reduced workers to %d for Colab", ve.MaxWorkers)
		}

		if ve.isT4Available() && ve.testNVENCEncoder() {
			ve.UseGPU = true
			ve.GPUDevice = "0"
			log.Printf("✅ T4 GPU detected and NVENC confirmed working")
		} else {
			ve.UseGPU = false
			log.Printf("⚠️ T4 GPU not detected or NVENC not working, using CPU")
		}

		// Disable multi-GPU in Colab
		ve.UseMultiGPU = false
	} else {
		log.Printf("🔧 Optimizing settings for local PC environment with multi-GPU support")

		// For local PC with multi-GPU, increase worker count
		if ve.UseMultiGPU && ve.MaxWorkers < 6 {
			ve.MaxWorkers = 6 // Increased for multi-GPU efficiency
			ve.WorkerPool = make(chan struct{}, ve.MaxWorkers)
			log.Printf("Increased workers to %d for multi-GPU processing", ve.MaxWorkers)
		}

		ve.debugGPUSetup()

		if ve.UseGPU && ve.checkNVIDIAPresence() {
			log.Printf("🎯 Local PC: NVIDIA GPU detected for multi-GPU setup")
		}
	}
}
