package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"performancereport/models"
	"performancereport/storage"
)

var aiHTTPClient = &http.Client{
	Timeout: 20 * time.Second,
}

type aiCreds struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl"`
}

// GetAICreds returns the saved AI credentials.
func GetAICreds() (map[string]string, error) {
	creds, err := loadAICreds()
	if err != nil {
		return map[string]string{}, nil
	}
	return map[string]string{
		"provider": creds.Provider,
		"apiKey":   creds.APIKey,
		"model":    creds.Model,
		"baseUrl":  creds.BaseURL,
	}, nil
}

// SaveAICreds stores AI credentials with automatic provider detection based on key format.
func SaveAICreds(provider, apiKey, model, baseURL string) error {
	apiKey = strings.TrimSpace(apiKey)
	// Auto-detect provider if obvious from key
	if strings.HasPrefix(apiKey, "AIzaSy") || strings.HasPrefix(apiKey, "AQ.") {
		if provider == "openai" || provider == "" {
			provider = "gemini"
		}
		if model == "" || strings.HasPrefix(model, "gpt-") || model == "gemini-2.0-flash" || model == "gemini-1.5-flash" {
			model = "gemini-3.8-flash"
		}
	} else if strings.HasPrefix(apiKey, "sk-") {
		if provider == "gemini" || provider == "" {
			provider = "openai"
		}
		if model == "" || strings.HasPrefix(model, "gemini-") {
			model = "gpt-4o-mini"
		}
	}
	creds := map[string]string{
		"provider": provider,
		"apiKey":   apiKey,
		"model":    model,
		"baseUrl":  baseURL,
	}
	return storage.WriteJSON(storage.AICredsPath(), &creds)
}

func loadAICreds() (*aiCreds, error) {
	var creds aiCreds
	if err := storage.ReadJSON(storage.AICredsPath(), &creds); err != nil {
		return nil, err
	}
	// Fallback to env var
	if creds.APIKey == "" {
		creds.APIKey = os.Getenv("GEMINI_API_KEY")
		if creds.APIKey != "" {
			creds.Provider = "gemini"
		}
	}
	if creds.APIKey == "" {
		creds.APIKey = os.Getenv("OPENAI_API_KEY")
		if creds.APIKey != "" {
			creds.Provider = "openai"
		}
	}
	return &creds, nil
}

// ImproveText calls the configured AI to improve/expand text.
func ImproveText(req *models.AIImproveRequest) (*models.AIImproveResponse, error) {
	creds, err := loadAICreds()
	if err != nil || creds.APIKey == "" {
		return nil, fmt.Errorf("AI not configured. Please enter your API Key in Settings")
	}

	prompt := buildPrompt(req)

	provider := strings.ToLower(strings.TrimSpace(creds.Provider))
	apiKey := strings.TrimSpace(creds.APIKey)

	// Auto-detect provider if mismatched
	if strings.HasPrefix(apiKey, "AIzaSy") || strings.HasPrefix(apiKey, "AQ.") {
		provider = "gemini"
	} else if strings.HasPrefix(apiKey, "sk-") {
		provider = "openai"
	}

	switch provider {
	case "openai":
		return callOpenAI(creds, prompt, req)
	case "gemini":
		return callGemini(creds, prompt, req)
	default:
		if strings.HasPrefix(apiKey, "AIzaSy") || strings.HasPrefix(apiKey, "AQ.") {
			return callGemini(creds, prompt, req)
		}
		return callOpenAI(creds, prompt, req)
	}
}

func buildPrompt(req *models.AIImproveRequest) string {
	detail := req.Detail
	if detail == "" {
		detail = "standard"
	}
	var sb strings.Builder
	sb.WriteString("You are a professional technical writer helping software developers write performance reports.\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Never fabricate technical details, measurements, or test results.\n")
	sb.WriteString("- Only expand what the developer has stated.\n")
	sb.WriteString("- Use professional, formal language suitable for performance appraisals.\n")
	sb.WriteString(fmt.Sprintf("- Output length: %s.\n\n", detail))

	switch req.Mode {
	case "improve":
		sb.WriteString("Improve the writing quality of the following text:\n\n")
	case "expand":
		sb.WriteString("Expand the following description into a detailed explanation:\n\n")
	case "activities":
		sb.WriteString("Write an 'Activities Performed' section for the following task:\n\n")
	case "issue":
		sb.WriteString("Write an 'Issue Identified' section for the following task:\n\n")
	case "solution":
		sb.WriteString("Write a 'Solution Implemented' section for the following task:\n\n")
	case "outcome":
		sb.WriteString("Write an 'Outcome' section for the following task:\n\n")
	case "bullets":
		sb.WriteString("Convert the following text to clear bullet points:\n\n")
	case "conclusion":
		sb.WriteString("Write a professional monthly performance report conclusion for:\n\n")
	case "title":
		sb.WriteString("Generate a concise, professional task title (max 10 words, no quotes) for the following work description:\n\n")
	case "challenges":
		sb.WriteString("Extract and format 'Challenges Faced' and 'Solution Implemented' bullet points strictly from the provided facts:\n\n")
	case "organize":
		sb.WriteString("Organize the following monthly work notes into structured Project Headings, Task Titles, and concise bullet descriptions without inventing any facts:\n\n")
	case "suggest_mapping":
		sb.WriteString("Suggest logical template field mappings (Cover Page, Employee Name, Employee Code, Appraiser Name, Month/Year, Projects, Table of Contents, Task Title, Task Description) based on these detected document properties:\n\n")
	default:
		sb.WriteString("Improve the following text:\n\n")
	}

	if req.Context != "" {
		sb.WriteString(fmt.Sprintf("Context: %s\n\n", req.Context))
	}
	sb.WriteString(req.Text)
	return sb.String()
}

func callOpenAI(creds *aiCreds, prompt string, req *models.AIImproveRequest) (*models.AIImproveResponse, error) {
	model := creds.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	baseURL := creds.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 1500,
	}
	b, _ := json.Marshal(body)
	httpReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(b))
	httpReq.Header.Set("Authorization", "Bearer "+creds.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := aiHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("network error connecting to OpenAI: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("OpenAI error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("OpenAI HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || len(result.Choices) == 0 {
		return nil, fmt.Errorf("invalid OpenAI response: %s", string(respBody))
	}
	return &models.AIImproveResponse{
		Original: req.Text,
		Improved: result.Choices[0].Message.Content,
		Mode:     req.Mode,
	}, nil
}

func executeGeminiCall(apiKey, model string, isOAuthToken bool, payload []byte) (int, []byte, error) {
	var url string
	if isOAuthToken {
		url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	} else {
		url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	}
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if isOAuthToken {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	}

	resp, err := aiHTTPClient.Do(httpReq)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

func callGemini(creds *aiCreds, prompt string, req *models.AIImproveRequest) (*models.AIImproveResponse, error) {
	model := creds.Model
	if model == "" || model == "gemini-2.0-flash" {
		model = "gemini-3.8-flash"
	}
	apiKey := strings.TrimSpace(creds.APIKey)
	isOAuthToken := strings.HasPrefix(apiKey, "ya29.")

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
	}
	payload, _ := json.Marshal(body)

	candidateModels := []string{model}
	for _, fb := range []string{"gemini-3.5-flash", "gemini-flash-latest", "gemini-3.8-flash"} {
		if !strings.EqualFold(fb, model) {
			candidateModels = append(candidateModels, fb)
		}
	}

	var lastErr error
	for _, m := range candidateModels {
		for attempt := 0; attempt < 2; attempt++ {
			statusCode, respBody, err := executeGeminiCall(apiKey, m, isOAuthToken, payload)
			if err != nil {
				lastErr = fmt.Errorf("network error connecting to Gemini API: %w", err)
				break
			}
			if statusCode == http.StatusOK {
				var result struct {
					Candidates []struct {
						Content struct {
							Parts []struct {
								Text string `json:"text"`
							} `json:"parts"`
						} `json:"content"`
					} `json:"candidates"`
				}
				if err := json.Unmarshal(respBody, &result); err == nil && len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
					return &models.AIImproveResponse{
						Original: req.Text,
						Improved: result.Candidates[0].Content.Parts[0].Text,
						Mode:     req.Mode,
					}, nil
				}
				return nil, fmt.Errorf("invalid Gemini response: %s", string(respBody))
			}

			if statusCode == 503 || statusCode == 429 {
				lastErr = fmt.Errorf("Gemini model %s is currently experiencing high demand (503)", m)
				if attempt == 0 {
					time.Sleep(800 * time.Millisecond)
					continue
				}
				break
			}

			var errResp struct {
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
					Status  string `json:"status"`
				} `json:"error"`
			}
			if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
				return nil, fmt.Errorf("Gemini error (%d %s): %s", errResp.Error.Code, errResp.Error.Status, errResp.Error.Message)
			}
			return nil, fmt.Errorf("Gemini HTTP error %d: %s", statusCode, string(respBody))
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w. Spikes in demand are temporary - please try again in a moment or select another model in Settings", lastErr)
	}
	return nil, fmt.Errorf("Gemini call failed")
}

// AITestResult holds diagnostic connection results
type AITestResult struct {
	Status    string `json:"status"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latencyMs"`
	Sample    string `json:"sample"`
}

// TestAIConnection performs a super-fast, low-token (max 3 tokens) ping test with a strict 10s timeout
func TestAIConnection() (*AITestResult, error) {
	creds, err := loadAICreds()
	if err != nil || strings.TrimSpace(creds.APIKey) == "" {
		return nil, fmt.Errorf("AI not configured. Please enter your API Key in Settings")
	}

	provider := strings.ToLower(strings.TrimSpace(creds.Provider))
	apiKey := strings.TrimSpace(creds.APIKey)
	if strings.HasPrefix(apiKey, "AIzaSy") || strings.HasPrefix(apiKey, "AQ.") {
		provider = "gemini"
	} else if strings.HasPrefix(apiKey, "sk-") {
		provider = "openai"
	}

	start := time.Now()
	testClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	if provider == "gemini" {
		model := creds.Model
		if model == "" || model == "gemini-2.0-flash" {
			model = "gemini-3.8-flash"
		}
		isOAuthToken := strings.HasPrefix(apiKey, "ya29.")

		body := map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]string{{"text": "Ping"}}},
			},
			"generationConfig": map[string]interface{}{
				"maxOutputTokens": 3,
				"temperature":     0.1,
			},
		}
		payload, _ := json.Marshal(body)

		candidateModels := []string{model}
		for _, fb := range []string{"gemini-3.5-flash", "gemini-flash-latest", "gemini-3.8-flash"} {
			if !strings.EqualFold(fb, model) {
				candidateModels = append(candidateModels, fb)
			}
		}

		var lastErr error
		for _, m := range candidateModels {
			for attempt := 0; attempt < 2; attempt++ {
				statusCode, respBody, err := executeGeminiCall(apiKey, m, isOAuthToken, payload)
				if err != nil {
					lastErr = fmt.Errorf("connection timeout or network error: %w", err)
					break
				}
				if statusCode == http.StatusOK {
					latency := time.Since(start).Milliseconds()
					resolvedModel := m
					if m != model {
						resolvedModel = fmt.Sprintf("%s (auto-fallback from %s due to high demand)", m, model)
					}
					return &AITestResult{
						Status:    "ok",
						Provider:  "Google Gemini",
						Model:     resolvedModel,
						LatencyMs: latency,
						Sample:    "Connection successful",
					}, nil
				}
				if statusCode == 503 || statusCode == 429 {
					lastErr = fmt.Errorf("Gemini model %s is currently experiencing high demand (503)", m)
					if attempt == 0 {
						time.Sleep(800 * time.Millisecond)
						continue
					}
					break
				}

				var errResp struct {
					Error struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
						Status  string `json:"status"`
					} `json:"error"`
				}
				if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
					return nil, fmt.Errorf("Gemini error (%d %s): %s", errResp.Error.Code, errResp.Error.Status, errResp.Error.Message)
				}
				return nil, fmt.Errorf("Gemini HTTP error %d: %s", statusCode, string(respBody))
			}
		}

		if lastErr != nil {
			return nil, fmt.Errorf("%w. Please wait a moment and try again, or switch model in Settings", lastErr)
		}
		return nil, fmt.Errorf("Gemini connection test failed")
	} else {
		baseURL := creds.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		model := creds.Model
		if model == "" {
			model = "gpt-4o-mini"
		}
		body := map[string]interface{}{
			"model": model,
			"messages": []map[string]string{
				{"role": "user", "content": "Ping"},
			},
			"max_tokens": 3,
		}
		b, _ := json.Marshal(body)
		httpReq, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+creds.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := testClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("connection timeout or network error: %w", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
				return nil, fmt.Errorf("OpenAI error: %s", errResp.Error.Message)
			}
			return nil, fmt.Errorf("OpenAI HTTP error %d: %s", resp.StatusCode, string(respBody))
		}

		latency := time.Since(start).Milliseconds()
		return &AITestResult{
			Status:    "ok",
			Provider:  "OpenAI",
			Model:     model,
			LatencyMs: latency,
			Sample:    "Connection successful",
		}, nil
	}
}
