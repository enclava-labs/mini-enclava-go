package rag

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

type QdrantClient struct {
	baseURL string
	client  *http.Client
}

type QdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float64      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

func NewQdrantClient(baseURL string, timeout time.Duration) *QdrantClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &QdrantClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *QdrantClient) EnsureCollection(ctx context.Context, collection string, vectorSize int) error {
	if strings.TrimSpace(collection) == "" {
		return fmt.Errorf("qdrant collection is required")
	}
	if vectorSize <= 0 {
		return fmt.Errorf("vector size must be positive")
	}
	status, _, err := c.do(ctx, http.MethodGet, "/collections/"+url.PathEscape(collection), nil)
	if err == nil && status < 300 {
		return nil
	}
	if err != nil && status != http.StatusNotFound {
		return err
	}
	if status != http.StatusNotFound && status != 0 {
		return fmt.Errorf("qdrant collection check failed with status %d", status)
	}

	body, err := json.Marshal(map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	})
	if err != nil {
		return err
	}
	status, resp, err := c.do(ctx, http.MethodPut, "/collections/"+url.PathEscape(collection), body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("qdrant create collection failed: status=%d body=%s", status, string(resp))
	}
	return nil
}

func (c *QdrantClient) UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error {
	body, err := json.Marshal(map[string]any{"points": points})
	if err != nil {
		return err
	}
	status, resp, err := c.do(ctx, http.MethodPut, "/collections/"+url.PathEscape(collection)+"/points?wait=true", body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("qdrant upsert failed: status=%d body=%s", status, string(resp))
	}
	return nil
}

func (c *QdrantClient) DeleteDocumentPoints(ctx context.Context, collection, documentID string) error {
	body, err := json.Marshal(map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "document_id", "match": map[string]any{"value": documentID}},
			},
		},
	})
	if err != nil {
		return err
	}
	status, resp, err := c.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(collection)+"/points/delete?wait=true", body)
	if err != nil {
		return err
	}
	if status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("qdrant delete document points failed: status=%d body=%s", status, string(resp))
	}
	return nil
}

func (c *QdrantClient) DeleteCollection(ctx context.Context, collection string) error {
	status, resp, err := c.do(ctx, http.MethodDelete, "/collections/"+url.PathEscape(collection), nil)
	if err != nil {
		return err
	}
	if status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("qdrant delete collection failed: status=%d body=%s", status, string(resp))
	}
	return nil
}

type QdrantSearchPoint struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

func (c *QdrantClient) Search(ctx context.Context, collection string, vector []float64, limit int, filter map[string]any) ([]QdrantSearchPoint, error) {
	if limit <= 0 {
		limit = 5
	}
	payload := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if len(filter) > 0 {
		payload["filter"] = filter
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	status, resp, err := c.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(collection)+"/points/search", body)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("qdrant search failed: status=%d body=%s", status, string(resp))
	}
	var envelope struct {
		Result []QdrantSearchPoint `json:"result"`
		Status string              `json:"status"`
	}
	if err := json.Unmarshal(resp, &envelope); err != nil {
		return nil, fmt.Errorf("decode qdrant search response: %w", err)
	}
	return envelope.Result, nil
}

func (c *QdrantClient) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("qdrant request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read qdrant response: %w", err)
	}
	return resp.StatusCode, respBody, nil
}
