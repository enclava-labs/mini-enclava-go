package llm

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

type OpenAICompatibleAdapter struct {
	baseURL      string
	apiKey       string
	client       *http.Client
	streamClient *http.Client
}

func NewOpenAICompatibleAdapter(baseURL, apiKey string, timeout time.Duration) *OpenAICompatibleAdapter {
	baseURL = strings.TrimSuffix(baseURL, "/")
	// Streaming client has no timeout: long-running SSE streams must not be killed.
	// Mitigation: Go's net/http cancels the request when the client disconnects (via r.Context()).
	// Known limitation: if the upstream hangs without the client disconnecting, the goroutine leaks.
	streamClient := &http.Client{
		Timeout: 0,
	}
	return &OpenAICompatibleAdapter{
		baseURL: baseURL,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: timeout,
		},
		streamClient: streamClient,
	}
}

func (a *OpenAICompatibleAdapter) ListModels(ctx context.Context) (ModelsResponse, error) {
	body, err := a.doJSON(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return ModelsResponse{}, err
	}
	var out ModelsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return ModelsResponse{}, fmt.Errorf("decode models response: %w", err)
	}
	return out, nil
}

func (a *OpenAICompatibleAdapter) GetModel(ctx context.Context, modelID string) (Model, error) {
	body, err := a.doJSON(ctx, http.MethodGet, "/models/"+url.PathEscape(modelID), nil)
	if err != nil {
		return Model{}, err
	}
	var out Model
	if err := json.Unmarshal(body, &out); err != nil {
		return Model{}, fmt.Errorf("decode model response: %w", err)
	}
	return out, nil
}

func (a *OpenAICompatibleAdapter) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	return a.doJSON(ctx, http.MethodPost, "/chat/completions", body)
}

func (a *OpenAICompatibleAdapter) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	return a.doJSON(ctx, http.MethodPost, "/embeddings", body)
}

func (a *OpenAICompatibleAdapter) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	url := a.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := a.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream chat stream request failed: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer func() {
			_ = resp.Body.Close()
		}()
		upstreamBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: upstreamBody}
	}
	return resp.Body, nil
}

func (a *OpenAICompatibleAdapter) doJSON(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := a.baseURL + path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	return respBody, nil
}
