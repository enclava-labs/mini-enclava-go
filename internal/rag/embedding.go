package rag

import (
	"context"
	"encoding/json"
	"fmt"

	"enclava-go/internal/llm"
)

const maxEmbeddingInputs = 2048

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func createEmbeddings(ctx context.Context, provider llm.Provider, model string, inputs []string, guard SpendGuard, apiKeyID int64) ([][]float64, embeddingResponse, error) {
	vectors := make([][]float64, len(inputs))
	var summed embeddingResponse
	var dim int
	for offset := 0; offset < len(inputs); offset += maxEmbeddingInputs {
		end := offset + maxEmbeddingInputs
		if end > len(inputs) {
			end = len(inputs)
		}
		batch := inputs[offset:end]
		if guard != nil && apiKeyID > 0 {
			if err := guard.Before(ctx, apiKeyID); err != nil {
				return nil, embeddingResponse{}, err
			}
		}
		batchVectors, resp, err := createEmbeddingBatch(ctx, provider, model, batch)
		if err != nil {
			return nil, embeddingResponse{}, err
		}
		if guard != nil && apiKeyID > 0 {
			if err := guard.RecordEmbedding(ctx, apiKeyID, model, resp.Usage.PromptTokens, resp.Usage.TotalTokens); err != nil {
				return nil, embeddingResponse{}, err
			}
		}
		for i, vec := range batchVectors {
			if dim == 0 {
				dim = len(vec)
			} else if len(vec) != dim {
				return nil, embeddingResponse{}, fmt.Errorf("embedding dimension mismatch: got %d, want %d", len(vec), dim)
			}
			vectors[offset+i] = vec
		}
		summed.Usage.PromptTokens += resp.Usage.PromptTokens
		summed.Usage.TotalTokens += resp.Usage.TotalTokens
	}
	return vectors, summed, nil
}

func createEmbeddingBatch(ctx context.Context, provider llm.Provider, model string, inputs []string) ([][]float64, embeddingResponse, error) {
	payload := map[string]any{
		"model": model,
		"input": inputs,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, embeddingResponse{}, fmt.Errorf("marshal embeddings request: %w", err)
	}
	respBody, err := provider.CreateEmbeddings(ctx, body)
	if err != nil {
		return nil, embeddingResponse{}, fmt.Errorf("provider embeddings: %w", err)
	}
	var resp embeddingResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, embeddingResponse{}, fmt.Errorf("decode embeddings response: %w", err)
	}
	if len(resp.Data) != len(inputs) {
		return nil, embeddingResponse{}, fmt.Errorf("embedding count mismatch: got %d, want %d", len(resp.Data), len(inputs))
	}
	vectors := make([][]float64, len(inputs))
	seen := make([]bool, len(inputs))
	for _, item := range resp.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, embeddingResponse{}, fmt.Errorf("embedding response has invalid index %d", item.Index)
		}
		if seen[item.Index] {
			return nil, embeddingResponse{}, fmt.Errorf("embedding response has duplicate index %d", item.Index)
		}
		if len(item.Embedding) == 0 {
			return nil, embeddingResponse{}, fmt.Errorf("embedding response has empty vector at index %d", item.Index)
		}
		seen[item.Index] = true
		vectors[item.Index] = item.Embedding
	}
	for i, ok := range seen {
		if !ok {
			return nil, embeddingResponse{}, fmt.Errorf("embedding response missing index %d", i)
		}
	}
	return vectors, resp, nil
}
