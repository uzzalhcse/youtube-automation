package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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
	_, err = io.Copy(audioWriter, audioFile)
	if err != nil {
		return "", fmt.Errorf("copying file content: %w", err)
	}

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

	// Create the HTTP request
	req, err := http.NewRequest("POST", baseURL, &requestBody)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", os.Getenv("USER_AGENT"))

	// Send the request
	resp, err := yt.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var transcriptResponse TranscriptResponse
	if err := json.Unmarshal(body, &transcriptResponse); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if len(transcriptResponse.SRT) == 0 || transcriptResponse.SRT == "" {
		return "", fmt.Errorf("no content in response")
	}

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
