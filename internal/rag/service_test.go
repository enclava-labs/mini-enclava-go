package rag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"enclava-go/internal/llm"
	"enclava-go/internal/store/sqlite"
)

type fakeEmbeddingProvider struct{}

func (f fakeEmbeddingProvider) ListModels(ctx context.Context) (llm.ModelsResponse, error) {
	return llm.ModelsResponse{}, nil
}

func (f fakeEmbeddingProvider) GetModel(ctx context.Context, modelID string) (llm.Model, error) {
	return llm.Model{}, nil
}

func (f fakeEmbeddingProvider) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	return nil, nil
}

func (f fakeEmbeddingProvider) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return nil, nil
}

func (f fakeEmbeddingProvider) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
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
			"embedding": []float64{float64(len(input)), float64(strings.Count(input, "enclava") + 1), 1},
		})
	}
	return json.Marshal(map[string]any{
		"object": "list",
		"data":   data,
		"usage":  map[string]any{"prompt_tokens": len(inputs), "total_tokens": len(inputs)},
	})
}

type fakeVectorStore struct {
	points map[string][]QdrantPoint
}

func newFakeVectorStore() *fakeVectorStore {
	return &fakeVectorStore{points: map[string][]QdrantPoint{}}
}

func (f *fakeVectorStore) EnsureCollection(ctx context.Context, collection string, vectorSize int) error {
	if _, ok := f.points[collection]; !ok {
		f.points[collection] = nil
	}
	return nil
}

func (f *fakeVectorStore) UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error {
	f.points[collection] = append(f.points[collection], points...)
	return nil
}

func (f *fakeVectorStore) DeleteDocumentPoints(ctx context.Context, collection, documentID string) error {
	kept := []QdrantPoint{}
	for _, point := range f.points[collection] {
		if point.Payload["document_id"] != documentID {
			kept = append(kept, point)
		}
	}
	f.points[collection] = kept
	return nil
}

func (f *fakeVectorStore) DeleteCollection(ctx context.Context, collection string) error {
	delete(f.points, collection)
	return nil
}

func (f *fakeVectorStore) Search(ctx context.Context, collection string, vector []float64, limit int, filter map[string]any) ([]QdrantSearchPoint, error) {
	out := []QdrantSearchPoint{}
	for _, point := range f.points[collection] {
		out = append(out, QdrantSearchPoint{ID: point.ID, Score: 0.99, Payload: point.Payload})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func TestServiceUploadSearchAndDelete(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "rag.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	vectorStore := newFakeVectorStore()
	svc := NewService(sqlite.NewRAGRepo(db.SQL), vectorStore, fakeEmbeddingProvider{}, "embed-test")
	access := Access{APIKeyID: createTestAPIKey(t, ctx, db)}

	collection, err := svc.CreateCollection(ctx, access, CreateCollectionInput{
		Name:         "Docs",
		ChunkSize:    100,
		ChunkOverlap: 10,
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	upload, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
		FileName: "note.txt",
		MimeType: "text/plain",
		Bytes:    []byte("enclava rag search content for integration testing"),
	})
	if err != nil {
		t.Fatalf("upload document: %v", err)
	}
	if upload.Document.Status != DocumentStatusReady {
		t.Fatalf("expected ready document, got %s", upload.Document.Status)
	}
	if len(upload.Chunks) == 0 {
		t.Fatalf("expected chunks")
	}

	results, err := svc.Search(ctx, access, SearchInput{CollectionID: collection.ID, Query: "enclava search", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].DocumentID != upload.Document.ID {
		t.Fatalf("unexpected search results: %#v", results)
	}

	if err := svc.DeleteDocument(ctx, access, upload.Document.ID); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if got := len(vectorStore.points[collection.QdrantCollection]); got != 0 {
		t.Fatalf("expected qdrant points removed, got %d", got)
	}
}

func TestServicePrivateCollectionACL(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "rag-acl.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	svc := NewService(sqlite.NewRAGRepo(db.SQL), newFakeVectorStore(), fakeEmbeddingProvider{}, "embed-test")
	ownerID := createTestAPIKey(t, ctx, db)
	otherID := createTestAPIKey(t, ctx, db)
	collection, err := svc.CreateCollection(ctx, Access{APIKeyID: ownerID}, CreateCollectionInput{Name: "Private"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if _, err := svc.GetCollection(ctx, Access{APIKeyID: otherID}, collection.ID); err == nil {
		t.Fatalf("expected private collection to be hidden from another api key")
	}
	if _, err := svc.GetCollection(ctx, Access{APIKeyID: otherID, IsAdmin: true}, collection.ID); err != nil {
		t.Fatalf("expected admin access: %v", err)
	}
}

type batchEmbeddingProvider struct {
	fakeEmbeddingProvider
	calls   [][]string
	badCall int
}

func (p *batchEmbeddingProvider) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	var payload struct {
		Input []string `json:"input"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	p.calls = append(p.calls, append([]string(nil), payload.Input...))
	if p.badCall > 0 && len(p.calls) == p.badCall {
		return []byte(`{"data":[{"index":0,"embedding":[]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`), nil
	}
	type item struct {
		Index int
		Text  string
	}
	items := make([]item, len(payload.Input))
	for i, text := range payload.Input {
		items[i] = item{Index: i, Text: text}
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	data := make([]map[string]any, 0, len(items))
	for _, it := range items {
		var n int
		if _, err := fmt.Sscanf(it.Text, "%d", &n); err != nil {
			n = -1
		}
		data = append(data, map[string]any{
			"index":     it.Index,
			"embedding": []float64{float64(n), float64(len(it.Text))},
		})
	}
	return json.Marshal(map[string]any{
		"data": data,
		"usage": map[string]any{
			"prompt_tokens": len(payload.Input),
			"total_tokens":  len(payload.Input) + 7,
		},
	})
}

type countingVectorStore struct {
	*fakeVectorStore
	upserts int
}

func (c *countingVectorStore) UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error {
	c.upserts++
	return c.fakeVectorStore.UpsertPoints(ctx, collection, points)
}

type recordingSpendGuard struct {
	failOnBefore int
	befores      int
	records      []struct{ prompt, total int }
}

func (g *recordingSpendGuard) Before(ctx context.Context, apiKeyID int64) error {
	g.befores++
	if g.failOnBefore > 0 && g.befores == g.failOnBefore {
		return errors.New("budget exceeded")
	}
	return nil
}

func (g *recordingSpendGuard) RecordEmbedding(ctx context.Context, apiKeyID int64, model string, promptTokens, totalTokens int) error {
	if model == "" || apiKeyID == 0 {
		return errors.New("missing usage identity")
	}
	g.records = append(g.records, struct{ prompt, total int }{promptTokens, totalTokens})
	return nil
}

func TestEmbeddingBatchesPreserveOrderAndSkipQdrantOnFailure(t *testing.T) {
	const chunks = 2049
	const chunkSize = 100
	text := repeatedNumberChunks(chunks, chunkSize)

	t.Run("two calls preserve order", func(t *testing.T) {
		ctx := context.Background()
		svc, vectorStore, provider, guard, collection, access := newBatchService(t, ctx, 0, 0)
		upload, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
			FileName: "batch.txt",
			MimeType: "text/plain",
			Bytes:    []byte(text),
		})
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		if len(provider.calls) != 2 || len(provider.calls[0]) != maxEmbeddingInputs || len(provider.calls[1]) != 1 {
			t.Fatalf("calls = %d parts, want 2048 and 1 (%d calls)", callSizes(provider.calls), len(provider.calls))
		}
		if provider.calls[0][0] != fmt.Sprintf("%0*d", chunkSize, 0) || provider.calls[1][0] != fmt.Sprintf("%0*d", chunkSize, chunks-1) {
			t.Fatalf("batch edges = %q and %q", provider.calls[0][0], provider.calls[1][0])
		}
		if len(guard.records) != 2 || guard.befores != 2 {
			t.Fatalf("guard befores=%d records=%v", guard.befores, guard.records)
		}
		if guard.records[0].prompt != maxEmbeddingInputs || guard.records[0].total != maxEmbeddingInputs+7 || guard.records[1].prompt != 1 || guard.records[1].total != 8 {
			t.Fatalf("usage records = %#v", guard.records)
		}
		points := vectorStore.points[collection.QdrantCollection]
		if vectorStore.upserts != 1 || len(points) != chunks || len(upload.Chunks) != chunks {
			t.Fatalf("upserts=%d points=%d chunks=%d", vectorStore.upserts, len(points), len(upload.Chunks))
		}
		for _, point := range points {
			content, _ := point.Payload["content"].(string)
			var n int
			if _, err := fmt.Sscanf(content, "%d", &n); err != nil {
				t.Fatalf("parse chunk %q: %v", content, err)
			}
			if len(point.Vector) != 2 || point.Vector[0] != float64(n) || point.Vector[1] != float64(len(content)) {
				t.Fatalf("order not preserved for %d: %#v", n, point.Vector)
			}
		}
		before := guard.befores
		if _, err := svc.Search(ctx, Access{IsAdmin: true}, SearchInput{CollectionID: collection.ID, Query: "0", Limit: 1}); err != nil {
			t.Fatalf("unbilled search: %v", err)
		}
		if guard.befores != before {
			t.Fatalf("api key 0 checked the guard: %d -> %d", before, guard.befores)
		}
	})

	t.Run("bad second batch writes no points", func(t *testing.T) {
		ctx := context.Background()
		svc, vectorStore, provider, guard, collection, access := newBatchService(t, ctx, 2, 0)
		_, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
			FileName: "batch.txt",
			MimeType: "text/plain",
			Bytes:    []byte(text),
		})
		if err == nil {
			t.Fatal("expected bad second batch to fail")
		}
		if len(provider.calls) != 2 || vectorStore.upserts != 0 || len(vectorStore.points[collection.QdrantCollection]) != 0 {
			t.Fatalf("calls=%d upserts=%d points=%d err=%v", len(provider.calls), vectorStore.upserts, len(vectorStore.points[collection.QdrantCollection]), err)
		}
		if len(guard.records) != 1 || guard.records[0].prompt != maxEmbeddingInputs {
			t.Fatalf("earlier batch was not recorded: %#v", guard.records)
		}
	})

	t.Run("budget failure skips later provider call", func(t *testing.T) {
		ctx := context.Background()
		svc, vectorStore, provider, guard, collection, access := newBatchService(t, ctx, 0, 2)
		_, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
			FileName: "batch.txt",
			MimeType: "text/plain",
			Bytes:    []byte(text),
		})
		if err == nil || !strings.Contains(err.Error(), "budget exceeded") {
			t.Fatalf("err=%v", err)
		}
		if len(provider.calls) != 1 || vectorStore.upserts != 0 || len(guard.records) != 1 {
			t.Fatalf("calls=%d upserts=%d records=%d", len(provider.calls), vectorStore.upserts, len(guard.records))
		}
	})
}

func newBatchService(t *testing.T, ctx context.Context, badCall, failBefore int) (*Service, *countingVectorStore, *batchEmbeddingProvider, *recordingSpendGuard, sqlite.RAGCollection, Access) {
	t.Helper()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "rag-batch.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	vectorStore := &countingVectorStore{fakeVectorStore: newFakeVectorStore()}
	provider := &batchEmbeddingProvider{badCall: badCall}
	guard := &recordingSpendGuard{failOnBefore: failBefore}
	svc := NewService(sqlite.NewRAGRepo(db.SQL), vectorStore, provider, "embed-test")
	svc.SetSpendGuard(guard)
	access := Access{APIKeyID: createTestAPIKey(t, ctx, db)}
	collection, err := svc.CreateCollection(ctx, access, CreateCollectionInput{
		Name:         "Batch",
		ChunkSize:    100,
		ChunkOverlap: 0,
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	return svc, vectorStore, provider, guard, collection, access
}

func repeatedNumberChunks(n, size int) string {
	var b strings.Builder
	b.Grow(n * size)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%0*d", size, i)
	}
	return b.String()
}

func callSizes(calls [][]string) []int {
	out := make([]int, len(calls))
	for i, call := range calls {
		out[i] = len(call)
	}
	return out
}

type failReplaceRepo struct {
	*sqlite.RAGRepo
}

func (f failReplaceRepo) ReplaceSourceDocument(ctx context.Context, doc sqlite.RAGDocument, blob []byte, chunks []sqlite.RAGDocumentChunk, deleteIDs []string) error {
	return errors.New("sqlite down")
}

func TestUploadSourceIDReplacesDocument(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "rag-source.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := sqlite.NewRAGRepo(db.SQL)
	vectorStore := newFakeVectorStore()
	svc := NewService(repo, vectorStore, fakeEmbeddingProvider{}, "embed-test")
	access := Access{APIKeyID: createTestAPIKey(t, ctx, db)}
	collection, err := svc.CreateCollection(ctx, access, CreateCollectionInput{Name: "Sources", ChunkSize: 100, ChunkOverlap: 0})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	versionA, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
		FileName:   "a.txt",
		MimeType:   "text/plain",
		Bytes:      []byte("version-A-unique-text"),
		SourceType: "slack",
		SourceID:   "thread-1",
	})
	if err != nil {
		t.Fatalf("upload A: %v", err)
	}
	oldPointIDs := map[string]bool{}
	for _, chunk := range versionA.Chunks {
		oldPointIDs[chunk.QdrantPointID] = true
	}
	legacyID := "ragdoc_legacy_duplicate"
	legacyPointID := "legacy-point-a"
	legacy := versionA.Document
	legacy.ID = legacyID
	legacy.FileName = "legacy.txt"
	if err := repo.CreateDocument(ctx, legacy, []byte("legacy-A-bytes"), []sqlite.RAGDocumentChunk{{
		ID:            "ragchunk_legacy",
		DocumentID:    legacyID,
		CollectionID:  collection.ID,
		ChunkIndex:    0,
		Content:       "legacy-A-chunk",
		TokenCount:    1,
		QdrantPointID: legacyPointID,
		Metadata:      map[string]any{},
	}}); err != nil {
		t.Fatalf("insert legacy duplicate: %v", err)
	}
	vectorStore.points[collection.QdrantCollection] = append(vectorStore.points[collection.QdrantCollection], QdrantPoint{
		ID:      legacyPointID,
		Vector:  []float64{1, 1, 1},
		Payload: map[string]any{"document_id": legacyID, "content": "legacy-A-chunk"},
	})

	versionB, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
		FileName:   "b.txt",
		MimeType:   "text/plain",
		Bytes:      []byte("version-B-unique-text"),
		SourceType: "slack",
		SourceID:   "thread-1",
		SourceURI:  "https://example.test/b",
	})
	if err != nil {
		t.Fatalf("upload B: %v", err)
	}
	if versionB.Document.ID != versionA.Document.ID {
		t.Fatalf("document id changed from %s to %s", versionA.Document.ID, versionB.Document.ID)
	}
	if versionB.Document.FileName != "b.txt" {
		t.Fatalf("file name = %s", versionB.Document.FileName)
	}
	docs, err := repo.ListDocuments(ctx, collection.ID, 100, 0)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != versionA.Document.ID {
		t.Fatalf("documents = %#v", docs)
	}
	if _, err := repo.GetDocument(ctx, legacyID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("legacy document err=%v", err)
	}
	chunks, err := repo.ListDocumentChunks(ctx, versionA.Document.ID)
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}
	joined := ""
	for _, chunk := range chunks {
		joined += chunk.Content
	}
	if !strings.Contains(joined, "version-B-unique-text") || strings.Contains(joined, "version-A-unique-text") || strings.Contains(joined, "legacy-A-chunk") {
		t.Fatalf("chunks = %q", joined)
	}
	blob, err := repo.GetDocumentBlob(ctx, versionA.Document.ID)
	if err != nil {
		t.Fatalf("blob: %v", err)
	}
	if string(blob) != "version-B-unique-text" {
		t.Fatalf("blob = %q", blob)
	}
	for _, point := range vectorStore.points[collection.QdrantCollection] {
		if oldPointIDs[point.ID] || point.ID == legacyPointID {
			t.Fatalf("stale point %s remains: %#v", point.ID, point.Payload)
		}
		content, _ := point.Payload["content"].(string)
		if strings.Contains(content, "version-A-unique-text") || strings.Contains(content, "legacy-A-chunk") {
			t.Fatalf("stale payload %q", content)
		}
		if !strings.Contains(content, "version-B-unique-text") {
			t.Fatalf("point content = %q", content)
		}
	}
	if len(vectorStore.points[collection.QdrantCollection]) == 0 {
		t.Fatal("expected replacement points")
	}

	first, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
		FileName: "one.txt",
		MimeType: "text/plain",
		Bytes:    []byte("plain upload one"),
	})
	if err != nil {
		t.Fatalf("empty source upload: %v", err)
	}
	second, err := svc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
		FileName: "two.txt",
		MimeType: "text/plain",
		Bytes:    []byte("plain upload two"),
	})
	if err != nil {
		t.Fatalf("second empty source upload: %v", err)
	}
	if first.Document.ID == second.Document.ID {
		t.Fatal("empty source id reused a document id")
	}

	failSvc := NewService(failReplaceRepo{RAGRepo: repo}, vectorStore, fakeEmbeddingProvider{}, "embed-test")
	beforeFail := append([]QdrantPoint(nil), vectorStore.points[collection.QdrantCollection]...)
	sawB := false
	for _, point := range beforeFail {
		if content, _ := point.Payload["content"].(string); strings.Contains(content, "version-B-unique-text") {
			sawB = true
		}
	}
	if !sawB {
		t.Fatal("fixture lost version B points before the failing replace")
	}
	_, err = failSvc.UploadDocument(ctx, access, collection.ID, UploadDocumentInput{
		FileName:   "c.txt",
		MimeType:   "text/plain",
		Bytes:      []byte("version-C-unique-text"),
		SourceType: "slack",
		SourceID:   "thread-1",
	})
	if err == nil || !strings.Contains(err.Error(), "sqlite down") {
		t.Fatalf("expected sqlite failure, got %v", err)
	}
	for _, point := range vectorStore.points[collection.QdrantCollection] {
		if point.Payload["document_id"] == versionA.Document.ID {
			t.Fatalf("source points remained after sqlite failure: %#v", point.Payload)
		}
		content, _ := point.Payload["content"].(string)
		if strings.Contains(content, "version-C-unique-text") || strings.Contains(content, "version-B-unique-text") {
			t.Fatalf("stale source point remained: %q", content)
		}
	}
	kept, err := repo.ListDocumentChunks(ctx, versionA.Document.ID)
	if err != nil {
		t.Fatalf("chunks after failed replace: %v", err)
	}
	keptText := ""
	for _, chunk := range kept {
		keptText += chunk.Content
	}
	if !strings.Contains(keptText, "version-B-unique-text") {
		t.Fatalf("sqlite changed after failed replace: %q", keptText)
	}
}

func createTestAPIKey(t *testing.T, ctx context.Context, db *sqlite.DB) int64 {
	t.Helper()
	id, err := sqlite.NewAPIKeysRepo(db.SQL).CreateAPIKey(ctx, "test", "enc_test_key_"+newID("secret"), "sha256", "", []string{"rag", "rag.manage"})
	if err != nil {
		t.Fatalf("create test api key: %v", err)
	}
	return id
}
