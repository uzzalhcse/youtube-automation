package elevenlabs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	BaseURL = "https://api.elevenlabs.io/v1"
)

type Proxy struct {
	Server   string
	Username string
	Password string
}

type Config struct {
	APIKey    string
	VoiceID   string
	TextFile  string
	OutputDir string
	Proxy     *Proxy
}

type TTSRequest struct {
	Text          string                 `json:"text"`
	ModelID       string                 `json:"model_id"`
	VoiceSettings map[string]interface{} `json:"voice_settings"`
}

type ElevenLabsClient struct {
	APIKey string
	Client *http.Client
}

func NewElevenLabsClient(apiKey string, proxy *Proxy) *ElevenLabsClient {
	client := &http.Client{Timeout: 30 * time.Second}

	// Configure proxy if provided
	if proxy != nil && proxy.Server != "" {
		proxyURL, err := url.Parse("http://" + proxy.Server)
		if err != nil {
			fmt.Printf("Warning: Invalid proxy server format: %v\n", err)
		} else {
			// Add authentication if provided
			if proxy.Username != "" {
				proxyURL.User = url.UserPassword(proxy.Username, proxy.Password)
			}

			transport := &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
			client.Transport = transport
			fmt.Printf("Using proxy: %s\n", proxy.Server)
		}
	}

	return &ElevenLabsClient{
		APIKey: apiKey,
		Client: client,
	}
}

func (c *ElevenLabsClient) TextToSpeech(text, voiceID string) ([]byte, error) {
	// Create request payload
	requestBody := TTSRequest{
		Text:    text,
		ModelID: "eleven_multilingual_v2", // Default model
		VoiceSettings: map[string]interface{}{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/text-to-speech/%s", BaseURL, voiceID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	// Set headers
	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.APIKey)

	fmt.Println("========APIKey", c.APIKey)

	// Make request
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Read response body
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	return audioData, nil
}

func (c *ElevenLabsClient) GetVoices() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/voices", BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("xi-api-key", c.APIKey)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	voices, ok := result["voices"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	var voiceList []map[string]interface{}
	for _, v := range voices {
		if voice, ok := v.(map[string]interface{}); ok {
			voiceList = append(voiceList, voice)
		}
	}

	return voiceList, nil
}

func readTextFromFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("error reading file %s: %v", filename, err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("file %s is empty", filename)
	}

	return text, nil
}

func ensureOutputDir(outputDir string) error {
	return os.MkdirAll(outputDir, 0755)
}

func saveAudioFile(audioData []byte, filename, outputDir string) error {
	if err := ensureOutputDir(outputDir); err != nil {
		return fmt.Errorf("error creating output directory: %v", err)
	}

	filepath := filepath.Join(outputDir, filename)
	return os.WriteFile(filepath, audioData, 0644)
}

func generateUniqueFilename(text string) string {
	timestamp := time.Now().Format("20060102_150405")
	// Truncate text for filename (max 50 chars)
	textPart := text
	if len(textPart) > 50 {
		textPart = textPart[:50]
	}
	// Remove problematic characters
	for _, char := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		textPart = string(bytes.ReplaceAll([]byte(textPart), []byte(char), []byte("_")))
	}
	return fmt.Sprintf("%s_%s.mp3", timestamp, textPart)
}

func loadEnvFile(filename string) (map[string]string, error) {
	env := make(map[string]string)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		env[key] = value
	}

	return env, scanner.Err()
}

func loadConfig() (*Config, error) {
	// Try to load .env file
	env, err := loadEnvFile(".env")
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %v", err)
	}

	config := &Config{}

	// Load API key
	config.APIKey = env["ELEVENLABS_API_KEY"]
	if config.APIKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY not found in .env file")
	}

	// Load voice ID (with default)
	config.VoiceID = env["VOICE_ID"]
	if config.VoiceID == "" {
		config.VoiceID = "j9jfwdrw7BRfcR43Qohk" // Default: Frederick Surrey
	}

	// Load text file path (with default)
	config.TextFile = env["TEXT_FILE"]
	if config.TextFile == "" {
		config.TextFile = "input.txt"
	}

	// Load output directory (with default)
	config.OutputDir = env["OUTPUT_DIR"]
	if config.OutputDir == "" {
		config.OutputDir = "elevenlabs/output"
	}

	// Load proxy configuration
	proxyServer := env["PROXY_SERVER"]
	if proxyServer != "" {
		config.Proxy = &Proxy{
			Server:   proxyServer,
			Username: env["PROXY_USERNAME"],
			Password: env["PROXY_PASSWORD"],
		}
	}

	return config, nil
}

func createSampleTextFile(filename string) error {
	sampleText := "Hello! This is a sample text for text-to-speech conversion. You can edit this file to change what will be spoken."
	return os.WriteFile(filename, []byte(sampleText), 0644)
}

func main() {
	// Load configuration from .env file
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Create client
	client := NewElevenLabsClient(config.APIKey, config.Proxy)

	// Check if input file exists, create sample if not
	if _, err := os.Stat(config.TextFile); os.IsNotExist(err) {
		fmt.Printf("Input file '%s' not found. Creating sample file...\n", config.TextFile)
		if err := createSampleTextFile(config.TextFile); err != nil {
			fmt.Printf("Error creating sample file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Sample file created. Please edit '%s' and run again.\n", config.TextFile)
		return
	}

	// Read text from file
	text, err := readTextFromFile(config.TextFile)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Reading text from file: %s\n", config.TextFile)
	fmt.Printf("Text content: \"%s\"\n", text)
	fmt.Printf("Using voice ID: %s\n", config.VoiceID)

	// Generate speech
	audioData, err := client.TextToSpeech(text, config.VoiceID)
	if err != nil {
		fmt.Printf("Error generating speech: %v\n", err)
		os.Exit(1)
	}

	// Save to file
	filename := generateUniqueFilename(text)
	if err := saveAudioFile(audioData, filename, config.OutputDir); err != nil {
		fmt.Printf("Error saving audio file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Audio saved successfully to: %s/%s\n", config.OutputDir, filename)
	fmt.Printf("File size: %.2f KB\n", float64(len(audioData))/1024)
}
