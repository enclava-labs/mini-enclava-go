package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type OpenAICompatibleAdapter struct {
	baseURL      string
	apiKey       string
	provider     string
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
		baseURL:  baseURL,
		apiKey:   apiKey,
		provider: "openai_compatible",
		client: &http.Client{
			Timeout: timeout,
		},
		streamClient: streamClient,
	}
}

func (a *OpenAICompatibleAdapter) WithProviderName(provider string) *OpenAICompatibleAdapter {
	provider = strings.TrimSpace(provider)
	if provider != "" {
		a.provider = provider
	}
	return a
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
	return NormalizeModels(out, a.provider), nil
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
	return NormalizeModel(out, a.provider), nil
}

func (a *OpenAICompatibleAdapter) Health(ctx context.Context) error {
	_, err := a.ListModels(ctx)
	return err
}

func (a *OpenAICompatibleAdapter) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	return a.doJSON(ctx, http.MethodPost, "/chat/completions", body)
}

func (a *OpenAICompatibleAdapter) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	return a.doJSON(ctx, http.MethodPost, "/embeddings", body)
}

func (a *OpenAICompatibleAdapter) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return a.doStream(ctx, http.MethodPost, "/chat/completions", body)
}

func (a *OpenAICompatibleAdapter) DoResponsesRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return a.doJSON(ctx, method, path, body)
}

func (a *OpenAICompatibleAdapter) DoResponsesStream(ctx context.Context, method, path string, body []byte) (io.ReadCloser, error) {
	return a.doStream(ctx, method, path, body)
}

func (a *OpenAICompatibleAdapter) DoPassthroughRequest(ctx context.Context, method, path string, body []byte, headers http.Header) (PassthroughResponse, error) {
	client := a.client
	if passthroughStreamClientSelected(path, headers, body) {
		client = a.streamClient
	}
	return doPassthroughHTTP(ctx, client, a.baseURL, a.apiKey, method, path, body, headers)
}

func (a *OpenAICompatibleAdapter) doStream(ctx context.Context, method, path string, body []byte) (io.ReadCloser, error) {
	requestURL := a.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
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

func doPassthroughHTTP(ctx context.Context, client *http.Client, baseURL, apiKey, method, path string, body []byte, headers http.Header) (PassthroughResponse, error) {
	requestURL, err := resolvePassthroughURL(baseURL, path)
	if err != nil {
		return PassthroughResponse{}, fmt.Errorf("build passthrough request: %w", err)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return PassthroughResponse{}, fmt.Errorf("build passthrough request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	copyPassthroughRequestHeaders(req.Header, headers)

	resp, err := client.Do(req)
	if err != nil {
		return PassthroughResponse{}, fmt.Errorf("upstream passthrough request failed: %w", err)
	}
	return PassthroughResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       resp.Body,
	}, nil
}

func copyPassthroughRequestHeaders(dst, src http.Header) {
	for name, values := range src {
		if !forwardPassthroughRequestHeader(name) {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
	if dst.Get("Accept") == "" {
		dst.Set("Accept", "application/json")
	}
}

func forwardPassthroughRequestHeader(name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	switch canonical {
	case "Accept", "Content-Type", "Idempotency-Key", "Openai-Beta", "Openai-Organization", "Openai-Project", "Openai-Version", "Openai-Intent", "X-Client-Request-Id":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(canonical), "openai-")
	}
}

func passthroughShouldUseStreamClient(path string, headers http.Header) bool {
	return passthroughStreamClientSelected(path, headers, nil)
}

func passthroughStreamClientSelected(path string, headers http.Header, body []byte) bool {
	if strings.Contains(strings.ToLower(headers.Get("Accept")), "text/event-stream") {
		return true
	}
	if strings.Contains(strings.ToLower(path), "stream=true") {
		return true
	}
	if !passthroughContentTypeAllowsStreamFlag(headers) {
		return false
	}
	return jsonBodyRequestsStream(body)
}

func passthroughContentTypeAllowsStreamFlag(headers http.Header) bool {
	contentType := strings.TrimSpace(headers.Get("Content-Type"))
	if contentType == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.EqualFold(mediaType, "application/json")
}

func jsonBodyRequestsStream(body []byte) bool {
	if len(bytes.TrimSpace(body)) == 0 {
		return false
	}
	var payload struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Stream
}

func resolvePassthroughURL(baseURL, rawPath string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("invalid provider base url")
	}
	ref, err := url.Parse(rawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid passthrough path")
	}
	if ref.IsAbs() || ref.Host != "" {
		return nil, fmt.Errorf("passthrough path escapes provider base")
	}
	rawPathPart := passthroughRawPath(rawPath)
	if rawPathPart == "" || rawPathPart[0] != '/' || ref.Path == "" || ref.Path[0] != '/' {
		return nil, fmt.Errorf("passthrough path must be absolute")
	}
	if strings.Contains(rawPathPart, `\`) || strings.Contains(ref.Path, `\`) || strings.Contains(rawPathPart, "://") {
		return nil, fmt.Errorf("passthrough path escapes provider base")
	}
	if strings.Contains(strings.ToLower(rawPathPart), "%2e%2e") || strings.Contains(ref.Path, "..") {
		return nil, fmt.Errorf("passthrough path escapes provider base")
	}
	basePath := base.Path
	if basePath == "" {
		basePath = "/"
	}
	joined := basePath
	if !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	joined += strings.TrimPrefix(ref.Path, "/")
	cleaned := path.Clean(joined)
	if !pathStaysUnderBase(basePath, cleaned) {
		return nil, fmt.Errorf("passthrough path escapes provider base")
	}
	resolved := *base
	resolved.Path = cleaned
	resolved.RawPath = ""
	resolved.RawQuery = ref.RawQuery
	resolved.Fragment = ""
	resolved.RawFragment = ""
	if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return nil, fmt.Errorf("passthrough path escapes provider base")
	}
	return &resolved, nil
}

func passthroughRawPath(rawPath string) string {
	if i := strings.IndexByte(rawPath, '?'); i >= 0 {
		rawPath = rawPath[:i]
	}
	if i := strings.IndexByte(rawPath, '#'); i >= 0 {
		rawPath = rawPath[:i]
	}
	return rawPath
}

func pathStaysUnderBase(basePath, cleaned string) bool {
	baseClean := path.Clean(basePath)
	if baseClean == "." || baseClean == "" {
		baseClean = "/"
	}
	cleaned = path.Clean(cleaned)
	if cleaned == "." || cleaned == "" || !strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "..") || strings.Contains(cleaned, `\`) {
		return false
	}
	if baseClean == "/" {
		return true
	}
	return cleaned == baseClean || strings.HasPrefix(cleaned, baseClean+"/")
}
