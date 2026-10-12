package llm

import (
	"context"
	"io"
	"testing"
	"time"
)

type cacheTestProvider struct {
	listCalls int
	models    ModelsResponse
}

func (p *cacheTestProvider) ListModels(ctx context.Context) (ModelsResponse, error) {
	p.listCalls++
	return p.models, nil
}

func (p *cacheTestProvider) GetModel(ctx context.Context, modelID string) (Model, error) {
	return Model{ID: modelID}, nil
}

func (p *cacheTestProvider) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	return nil, nil
}

func (p *cacheTestProvider) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return nil, nil
}

func (p *cacheTestProvider) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	return nil, nil
}

func TestModelCacheProviderCachesAndInvalidatesModels(t *testing.T) {
	upstream := &cacheTestProvider{
		models: ModelsResponse{Data: []Model{{ID: "chat-model", Tasks: []string{"chat"}}}},
	}
	provider := NewModelCacheProvider(upstream, time.Minute, "test-provider")

	first, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models first: %v", err)
	}
	second, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models second: %v", err)
	}
	if upstream.listCalls != 1 {
		t.Fatalf("expected one upstream call before invalidation, got %d", upstream.listCalls)
	}
	if first.Data[0].Provider != "test-provider" || !second.Data[0].SupportsStreaming {
		t.Fatalf("expected normalized cached model metadata: %#v %#v", first.Data[0], second.Data[0])
	}
	model, err := provider.GetModel(context.Background(), "chat-model")
	if err != nil {
		t.Fatalf("get cached model: %v", err)
	}
	if model.ID != "chat-model" {
		t.Fatalf("unexpected cached model: %#v", model)
	}

	provider.InvalidateModelCache()
	if _, err := provider.ListModels(context.Background()); err != nil {
		t.Fatalf("list models after invalidation: %v", err)
	}
	if upstream.listCalls != 2 {
		t.Fatalf("expected upstream call after invalidation, got %d", upstream.listCalls)
	}
}
