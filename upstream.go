package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// User-Agent fingerprints mimicking the official clients (see free-buff-lol/proxy.js).
const (
	uaJSON = "Bun/1.3.11"
	uaCLI  = "Freebuff-CLI/0.0.105"
	uaChat = "ai-sdk/openai-compatible/0.0.0-test/codebuff ai-sdk/provider-utils/3.0.20 runtime/browser"
)

type UpstreamClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewUpstreamClient(cfg Config) *UpstreamClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.HTTPProxy != "" {
		if proxyURL, err := url.Parse(cfg.HTTPProxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &UpstreamClient{
		baseURL: cfg.UpstreamBaseURL,
		httpClient: &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: transport,
		},
	}
}

// StartRun opens an agent run. ancestorRunIds links child runs to their parent
// (context-pruner child, gemini chat child) as the CLI does.
func (c *UpstreamClient) StartRun(ctx context.Context, authToken, actingUserID, agentID string, ancestorRunIds []string) (string, error) {
	payload := map[string]any{
		"action":  "START",
		"agentId": agentID,
	}
	if len(ancestorRunIds) > 0 {
		payload["ancestorRunIds"] = ancestorRunIds
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal start run request: %w", err)
	}

	resp, err := c.doJSON(ctx, authToken, actingUserID, "/api/v1/agent-runs", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read start run response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("start run failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", fmt.Errorf("decode start run response: %w", err)
	}
	if strings.TrimSpace(parsed.RunID) == "" {
		return "", fmt.Errorf("start run response missing runId: %s", strings.TrimSpace(string(responseBody)))
	}

	return parsed.RunID, nil
}

func (c *UpstreamClient) FinishRun(ctx context.Context, authToken, actingUserID, runID string, totalSteps int) error {
	payload := map[string]any{
		"action":        "FINISH",
		"runId":         runID,
		"status":        "completed",
		"totalSteps":    totalSteps,
		"directCredits": 0,
		"totalCredits":  0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal finish run request: %w", err)
	}

	resp, err := c.doJSON(ctx, authToken, actingUserID, "/api/v1/agent-runs", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read finish run response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("finish run failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

// RecordRunStep logs a step against a run, matching the CLI's bookkeeping so
// runs look like genuine agent activity.
func (c *UpstreamClient) RecordRunStep(ctx context.Context, authToken, actingUserID, runID string, stepNumber int, childRunIds []string, messageID *string, startTime string) error {
	if startTime == "" {
		startTime = time.Now().UTC().Format(time.RFC3339)
	}
	payload := map[string]any{
		"stepNumber":  stepNumber,
		"credits":     0,
		"childRunIds": childRunIds,
		"messageId":   messageID,
		"status":      "completed",
		"startTime":   startTime,
	}
	if childRunIds == nil {
		payload["childRunIds"] = []string{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal run step request: %w", err)
	}

	path := "/api/v1/agent-runs/" + url.PathEscape(runID) + "/steps"
	resp, err := c.doJSON(ctx, authToken, actingUserID, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read run step response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("record run step failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (c *UpstreamClient) ChatCompletions(ctx context.Context, authToken, actingUserID string, body []byte) (*http.Response, []byte, error) {
	requestURL, err := url.JoinPath(c.baseURL, "/api/v1/chat/completions")
	if err != nil {
		return nil, nil, fmt.Errorf("build upstream url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	// Accept-Encoding left to Go's transport so gzip responses are transparently
	// decompressed; setting it manually would disable that.
	req.Header.Set("User-Agent", uaChat)
	if actingUserID != "" {
		req.Header.Set("x-freebuff-acting-user-id", actingUserID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("send upstream request: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil, nil
	}

	responseBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, nil, fmt.Errorf("read upstream error response: %w", readErr)
	}
	return resp, responseBody, nil
}

// FetchUserID calls /api/v1/me to get the user's ID for the
// x-freebuff-acting-user-id header that gates free-mode chat.
func (c *UpstreamClient) FetchUserID(ctx context.Context, authToken string) (string, error) {
	requestURL := strings.TrimRight(c.baseURL, "/") + "/api/v1/me?fields=id,email"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("User-Agent", uaJSON)
	req.Header.Set("Accept", "*/*")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch user: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch user failed %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode user: %w", err)
	}
	return parsed.ID, nil
}

func (c *UpstreamClient) doJSON(ctx context.Context, authToken, actingUserID, path string, body []byte) (*http.Response, error) {
	requestURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("build upstream url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", uaJSON)
	if actingUserID != "" {
		req.Header.Set("x-freebuff-acting-user-id", actingUserID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send upstream request: %w", err)
	}
	return resp, nil
}

