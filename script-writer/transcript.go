package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type TranscriptPayload struct {
	AudioPath string `json:"audio_path"` // Changed to file path instead of base64
	Language  string `json:"language"`
	OutputSrt bool   `json:"output_srt"`
}

type TranscriptResponse struct {
	Text      string  `json:"text"`
	SRT       string  `json:"srt,omitempty"`
	Language  string  `json:"language,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Timestamp string  `json:"timestamp"`
	Filename  string  `json:"filename"`
	Format    string  `json:"format"` // "text" or "srt"
}

func (yt *YtAutomation) callTranscriptAPI(payload TranscriptPayload) (string, error) {
	// Debug environment variable
	apiURL := os.Getenv("TRANSCRIPT_SERVER_API_URL")
	fmt.Printf("TRANSCRIPT_SERVER_API_URL: '%s'\n", apiURL)
	if apiURL == "" {
		return "", fmt.Errorf("TRANSCRIPT_SERVER_API_URL environment variable is not set")
	}

	// Check if file exists and get its size
	fileInfo, err := os.Stat(payload.AudioPath)
	if err != nil {
		return "", fmt.Errorf("audio file stat error: %w", err)
	}
	fmt.Printf("Audio file: %s, Size: %d bytes\n", payload.AudioPath, fileInfo.Size())

	// Create a buffer to store the form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add the audio file
	audioFile, err := os.Open(payload.AudioPath)
	if err != nil {
		return "", fmt.Errorf("opening audio file: %w", err)
	}
	defer audioFile.Close()

	// Create form file field
	audioWriter, err := writer.CreateFormFile("audio", filepath.Base(payload.AudioPath))
	if err != nil {
		return "", fmt.Errorf("creating form file field: %w", err)
	}

	// Copy file content to form
	bytesWritten, err := io.Copy(audioWriter, audioFile)
	if err != nil {
		return "", fmt.Errorf("copying file content: %w", err)
	}
	fmt.Printf("Bytes written to form: %d\n", bytesWritten)

	// Add other form fields
	err = writer.WriteField("language", payload.Language)
	if err != nil {
		return "", fmt.Errorf("writing language field: %w", err)
	}

	err = writer.WriteField("output_srt", strconv.FormatBool(payload.OutputSrt))
	if err != nil {
		return "", fmt.Errorf("writing output_srt field: %w", err)
	}

	// Close the writer to finalize the form
	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("closing form writer: %w", err)
	}

	fmt.Printf("Total request body size: %d bytes\n", requestBody.Len())

	url := fmt.Sprintf("%s/transcribe", apiURL)
	fmt.Printf("Request URL: %s\n", url)

	// Create the HTTP request
	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	// Set headers
	contentType := writer.FormDataContentType()
	req.Header.Set("Content-Type", contentType)
	fmt.Printf("Content-Type: %s\n", contentType)

	// Only set User-Agent if it's not empty
	userAgent := os.Getenv("USER_AGENT")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
		fmt.Printf("User-Agent: %s\n", userAgent)
	} else {
		fmt.Println("User-Agent not set (empty environment variable)")
	}

	fmt.Printf("Request headers: %+v\n", req.Header)

	// Use longer timeout for large files
	client := &http.Client{
		Timeout: 10 * time.Minute, // Increased timeout
	}

	fmt.Println("Sending HTTP request...")
	startTime := time.Now()

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("HTTP request failed after %v: %v\n", time.Since(startTime), err)

		// Check for specific error types
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				fmt.Println("Error type: Network timeout")
			}
			if netErr.Temporary() {
				fmt.Println("Error type: Temporary network error")
			}
		}

		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Response received after %v, Status: %d\n", time.Since(startTime), resp.StatusCode)
	fmt.Printf("Response headers: %+v\n", resp.Header)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error response body: %s\n", string(body))
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	fmt.Printf("Response body length: %d bytes\n", len(body))

	var transcriptResponse TranscriptResponse
	if err := json.Unmarshal(body, &transcriptResponse); err != nil {
		fmt.Printf("JSON unmarshal error. Response body: %s\n", string(body))
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(transcriptResponse.SRT) == 0 || transcriptResponse.SRT == "" {
		fmt.Printf("Empty SRT in response: %+v\n", transcriptResponse)
		return "", fmt.Errorf("no content in response")
	}

	fmt.Printf("Successfully received SRT content, length: %d\n", len(transcriptResponse.SRT))
	return transcriptResponse.SRT, nil
}
func (yt *YtAutomation) GenerateSRT(payload TranscriptPayload) (string, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := yt.callTranscriptAPI(payload)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Exponential backoff: 1s, 2s, 4s, 8s...
		backoffDuration := time.Duration(1<<attempt) * time.Second
		fmt.Printf("API call failed (attempt %d/%d), retrying in %v: %v\n",
			attempt+1, maxRetries, backoffDuration, err)

		if attempt < maxRetries-1 {
			time.Sleep(backoffDuration)
		}
	}

	return "", fmt.Errorf("API call failed after %d attempts, last error: %w", maxRetries, lastErr)
}
