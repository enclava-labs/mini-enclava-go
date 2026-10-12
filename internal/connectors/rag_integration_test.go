package connectors

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"enclava-go/internal/llm"
	"enclava-go/internal/rag"
	"enclava-go/internal/store/sqlite"
)

type connectorEmbeddingProvider struct{}

func (p connectorEmbeddingProvider) ListModels(ctx context.Context) (llm.ModelsResponse, error) {
	return llm.ModelsResponse{}, nil
}

func (p connectorEmbeddingProvider) GetModel(ctx context.Context, modelID string) (llm.Model, error) {
	return llm.Model{}, nil
}

func (p connectorEmbeddingProvider) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	return nil, nil
}

func (p connectorEmbeddingProvider) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return nil, nil
}

func (p connectorEmbeddingProvider) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	var payload struct {
		Input any `json:"input"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	inputs := []string{}
	switch v := payload.Input.(type) {
	case string:
		inputs = append(inputs, v)
	case []any:
		for _, item := range v {
			inputs = append(inputs, item.(string))
		}
	}
	data := make([]map[string]any, 0, len(inputs))
	for i, input := range inputs {
		data = append(data, map[string]any{
			"index":     i,
			"embedding": []float64{float64(len(input)), float64(strings.Count(input, "connector") + 1), 1},
		})
	}
	return json.Marshal(map[string]any{
		"object": "list",
		"data":   data,
		"usage":  map[string]any{"prompt_tokens": len(inputs), "total_tokens": len(inputs)},
	})
}

type connectorVectorStore struct {
	points map[string][]rag.QdrantPoint
}

func newConnectorVectorStore() *connectorVectorStore {
	return &connectorVectorStore{points: map[string][]rag.QdrantPoint{}}
}

func (s *connectorVectorStore) EnsureCollection(ctx context.Context, collection string, vectorSize int) error {
	if _, ok := s.points[collection]; !ok {
		s.points[collection] = nil
	}
	return nil
}

func (s *connectorVectorStore) UpsertPoints(ctx context.Context, collection string, points []rag.QdrantPoint) error {
	s.points[collection] = append(s.points[collection], points...)
	return nil
}

func (s *connectorVectorStore) DeleteDocumentPoints(ctx context.Context, collection, documentID string) error {
	kept := []rag.QdrantPoint{}
	for _, point := range s.points[collection] {
		if point.Payload["document_id"] != documentID {
			kept = append(kept, point)
		}
	}
	s.points[collection] = kept
	return nil
}

func (s *connectorVectorStore) DeleteCollection(ctx context.Context, collection string) error {
	delete(s.points, collection)
	return nil
}

func (s *connectorVectorStore) Search(ctx context.Context, collection string, vector []float64, limit int, filter map[string]any) ([]rag.QdrantSearchPoint, error) {
	out := []rag.QdrantSearchPoint{}
	for _, point := range s.points[collection] {
		out = append(out, rag.QdrantSearchPoint{ID: point.ID, Score: 0.99, Payload: point.Payload})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func TestConnectorSyncIndexesDocumentsIntoSearchableRAG(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "connector-rag.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	apiKeyID, err := sqlite.NewAPIKeysRepo(db.SQL).CreateAPIKey(ctx, "connector-rag", "enc_test_connector_rag_secret", "sha256", "", []string{"rag", "rag.manage", "connectors.manage"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	ragSvc := rag.NewService(sqlite.NewRAGRepo(db.SQL), newConnectorVectorStore(), connectorEmbeddingProvider{}, "embed-test")
	access := rag.Access{APIKeyID: apiKeyID}
	collection, err := ragSvc.CreateCollection(ctx, access, rag.CreateCollectionInput{
		Name:         "Connector Search",
		ChunkSize:    100,
		ChunkOverlap: 10,
	})
	if err != nil {
		t.Fatalf("create rag collection: %v", err)
	}
	connectorSvc := NewService(sqlite.NewConnectorsRepo(db.SQL), ragSvc)
	source, err := connectorSvc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeSlack,
		Name:               "Slack Knowledge",
		TargetCollectionID: collection.ID,
		Config: map[string]any{
			"documents": []any{
				map[string]any{
					"file_name":  "thread.txt",
					"mime_type":  "text/plain",
					"content":    "connector rag searchable acceptance phrase",
					"source_id":  "thread-acceptance",
					"source_uri": "slack://thread-acceptance",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create connector source: %v", err)
	}
	if _, err := connectorSvc.SyncSource(ctx, Access{APIKeyID: apiKeyID}, source.ID); err != nil {
		t.Fatalf("sync connector source: %v", err)
	}

	results, err := ragSvc.Search(ctx, access, rag.SearchInput{
		CollectionID: collection.ID,
		Query:        "acceptance phrase",
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("search rag: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected connector-ingested RAG result")
	}
	if !strings.Contains(results[0].Content, "connector rag searchable acceptance phrase") {
		t.Fatalf("unexpected search content: %#v", results[0])
	}
	if results[0].Metadata["connector_source_id"] != source.ID || results[0].Metadata["connector_type"] != SourceTypeSlack {
		t.Fatalf("missing connector metadata in search result: %#v", results[0].Metadata)
	}
}
