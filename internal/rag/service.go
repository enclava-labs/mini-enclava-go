package rag

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"enclava-go/internal/llm"
	"enclava-go/internal/store/sqlite"
)

type Repository interface {
	CreateCollection(ctx context.Context, c sqlite.RAGCollection) error
	ListCollections(ctx context.Context) ([]sqlite.RAGCollection, error)
	GetCollection(ctx context.Context, id string) (sqlite.RAGCollection, error)
	UpdateCollection(ctx context.Context, c sqlite.RAGCollection) error
	DeleteCollection(ctx context.Context, id string) error
	ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]sqlite.RAGDocument, error)
	GetDocument(ctx context.Context, id string) (sqlite.RAGDocument, error)
	CreateDocument(ctx context.Context, doc sqlite.RAGDocument, blob []byte, chunks []sqlite.RAGDocumentChunk) error
	ListDocumentsBySource(ctx context.Context, collectionID, sourceType, sourceID string) ([]sqlite.RAGDocument, error)
	ReplaceSourceDocument(ctx context.Context, doc sqlite.RAGDocument, blob []byte, chunks []sqlite.RAGDocumentChunk, deleteIDs []string) error
	ReplaceDocumentProcessing(ctx context.Context, doc sqlite.RAGDocument, chunks []sqlite.RAGDocumentChunk) error
	GetDocumentBlob(ctx context.Context, documentID string) ([]byte, error)
	ListDocumentChunks(ctx context.Context, documentID string) ([]sqlite.RAGDocumentChunk, error)
	DeleteDocument(ctx context.Context, id string) error
}

type VectorStore interface {
	EnsureCollection(ctx context.Context, collection string, vectorSize int) error
	UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error
	DeleteDocumentPoints(ctx context.Context, collection, documentID string) error
	DeleteCollection(ctx context.Context, collection string) error
	Search(ctx context.Context, collection string, vector []float64, limit int, filter map[string]any) ([]QdrantSearchPoint, error)
}

type Access struct {
	APIKeyID int64
	IsAdmin  bool
}

// SpendGuard is an optional budget check and usage recorder for embedding calls.
// A nil guard skips both. apiKeyID 0 is not checked or recorded.
type SpendGuard interface {
	Before(ctx context.Context, apiKeyID int64) error
	RecordEmbedding(ctx context.Context, apiKeyID int64, model string, promptTokens, totalTokens int) error
}

type Service struct {
	repo                  Repository
	vectorStore           VectorStore
	provider              llm.Provider
	defaultEmbeddingModel string
	spendGuard            SpendGuard
}

func NewService(repo Repository, vectorStore VectorStore, provider llm.Provider, defaultEmbeddingModel string) *Service {
	if strings.TrimSpace(defaultEmbeddingModel) == "" {
		defaultEmbeddingModel = "nomic-embed-text"
	}
	return &Service{
		repo:                  repo,
		vectorStore:           vectorStore,
		provider:              provider,
		defaultEmbeddingModel: defaultEmbeddingModel,
	}
}

func (s *Service) SetSpendGuard(guard SpendGuard) {
	s.spendGuard = guard
}

func (s *Service) CreateCollection(ctx context.Context, access Access, input CreateCollectionInput) (sqlite.RAGCollection, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return sqlite.RAGCollection{}, errors.New("name is required")
	}
	chunkSize, chunkOverlap, err := normalizeChunkSettings(input.ChunkSize, input.ChunkOverlap)
	if err != nil {
		return sqlite.RAGCollection{}, err
	}
	embeddingModel := strings.TrimSpace(input.EmbeddingModel)
	if embeddingModel == "" {
		embeddingModel = s.defaultEmbeddingModel
	}
	id := newID("ragcol")
	c := sqlite.RAGCollection{
		ID:               id,
		Name:             name,
		Description:      strings.TrimSpace(input.Description),
		QdrantCollection: "rag_" + safeCollectionName(id),
		EmbeddingModel:   embeddingModel,
		ChunkSize:        chunkSize,
		ChunkOverlap:     chunkOverlap,
		ACL:              normalizeACL(input.ACL),
		Metadata:         nonNilMap(input.Metadata),
		IsActive:         true,
	}
	if access.APIKeyID > 0 {
		owner := access.APIKeyID
		c.OwnerAPIKeyID = &owner
	}
	if err := s.repo.CreateCollection(ctx, c); err != nil {
		return sqlite.RAGCollection{}, err
	}
	return s.repo.GetCollection(ctx, id)
}

func (s *Service) ListCollections(ctx context.Context, access Access) ([]sqlite.RAGCollection, error) {
	collections, err := s.repo.ListCollections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sqlite.RAGCollection, 0, len(collections))
	for _, c := range collections {
		if collectionReadable(c, access) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Service) GetCollection(ctx context.Context, access Access, id string) (sqlite.RAGCollection, error) {
	c, err := s.repo.GetCollection(ctx, id)
	if err != nil {
		return sqlite.RAGCollection{}, err
	}
	if !collectionReadable(c, access) {
		return sqlite.RAGCollection{}, sql.ErrNoRows
	}
	return c, nil
}

func (s *Service) UpdateCollection(ctx context.Context, access Access, id string, input UpdateCollectionInput) (sqlite.RAGCollection, error) {
	c, err := s.repo.GetCollection(ctx, id)
	if err != nil {
		return sqlite.RAGCollection{}, err
	}
	if !collectionManageable(c, access) {
		return sqlite.RAGCollection{}, sql.ErrNoRows
	}
	if input.Name != nil {
		c.Name = strings.TrimSpace(*input.Name)
	}
	if c.Name == "" {
		return sqlite.RAGCollection{}, errors.New("name is required")
	}
	if input.Description != nil {
		c.Description = strings.TrimSpace(*input.Description)
	}
	if input.EmbeddingModel != nil {
		c.EmbeddingModel = strings.TrimSpace(*input.EmbeddingModel)
	}
	if c.EmbeddingModel == "" {
		return sqlite.RAGCollection{}, errors.New("embedding_model is required")
	}
	if input.ChunkSize != nil || input.ChunkOverlap != nil {
		chunkSize := c.ChunkSize
		chunkOverlap := c.ChunkOverlap
		if input.ChunkSize != nil {
			chunkSize = *input.ChunkSize
		}
		if input.ChunkOverlap != nil {
			chunkOverlap = *input.ChunkOverlap
		}
		chunkSize, chunkOverlap, err = normalizeChunkSettings(chunkSize, chunkOverlap)
		if err != nil {
			return sqlite.RAGCollection{}, err
		}
		c.ChunkSize = chunkSize
		c.ChunkOverlap = chunkOverlap
	}
	if input.ACL != nil {
		c.ACL = normalizeACL(input.ACL)
	}
	if input.Metadata != nil {
		c.Metadata = input.Metadata
	}
	if input.IsActive != nil {
		c.IsActive = *input.IsActive
	}
	if err := s.repo.UpdateCollection(ctx, c); err != nil {
		return sqlite.RAGCollection{}, err
	}
	return s.repo.GetCollection(ctx, id)
}

func (s *Service) DeleteCollection(ctx context.Context, access Access, id string) error {
	c, err := s.repo.GetCollection(ctx, id)
	if err != nil {
		return err
	}
	if !collectionManageable(c, access) {
		return sql.ErrNoRows
	}
	if s.vectorStore != nil {
		if err := s.vectorStore.DeleteCollection(ctx, c.QdrantCollection); err != nil {
			return err
		}
	}
	return s.repo.DeleteCollection(ctx, id)
}

func (s *Service) UploadDocument(ctx context.Context, access Access, collectionID string, input UploadDocumentInput) (UploadDocumentOutput, error) {
	c, err := s.GetCollection(ctx, access, collectionID)
	if err != nil {
		return UploadDocumentOutput{}, err
	}
	if !collectionManageable(c, access) {
		return UploadDocumentOutput{}, sql.ErrNoRows
	}
	return s.processNewDocument(ctx, c, input, access.APIKeyID)
}

func (s *Service) ListDocuments(ctx context.Context, access Access, collectionID string, limit, offset int) ([]sqlite.RAGDocument, error) {
	c, err := s.GetCollection(ctx, access, collectionID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListDocuments(ctx, c.ID, limit, offset)
}

func (s *Service) DownloadDocument(ctx context.Context, access Access, documentID string) (sqlite.RAGDocument, []byte, error) {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return sqlite.RAGDocument{}, nil, err
	}
	if _, err := s.GetCollection(ctx, access, doc.CollectionID); err != nil {
		return sqlite.RAGDocument{}, nil, err
	}
	blob, err := s.repo.GetDocumentBlob(ctx, doc.ID)
	if err != nil {
		return sqlite.RAGDocument{}, nil, err
	}
	return doc, blob, nil
}

func (s *Service) DeleteDocument(ctx context.Context, access Access, documentID string) error {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return err
	}
	c, err := s.repo.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return err
	}
	if !collectionManageable(c, access) {
		return sql.ErrNoRows
	}
	if s.vectorStore != nil {
		if err := s.vectorStore.DeleteDocumentPoints(ctx, c.QdrantCollection, doc.ID); err != nil {
			return err
		}
	}
	return s.repo.DeleteDocument(ctx, doc.ID)
}

func (s *Service) ReprocessDocument(ctx context.Context, access Access, documentID string) (UploadDocumentOutput, error) {
	doc, blob, err := s.DownloadDocument(ctx, access, documentID)
	if err != nil {
		return UploadDocumentOutput{}, err
	}
	c, err := s.repo.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return UploadDocumentOutput{}, err
	}
	if !collectionManageable(c, access) {
		return UploadDocumentOutput{}, sql.ErrNoRows
	}
	if s.vectorStore != nil {
		if err := s.vectorStore.DeleteDocumentPoints(ctx, c.QdrantCollection, doc.ID); err != nil {
			return UploadDocumentOutput{}, err
		}
	}
	prepared, err := s.prepareDocumentContent(ctx, c, &doc, blob, access.APIKeyID)
	if err != nil {
		return UploadDocumentOutput{}, err
	}
	if err := s.storePoints(ctx, c.QdrantCollection, prepared.points); err != nil {
		return UploadDocumentOutput{}, err
	}
	applyPreparedDocument(&doc, prepared)
	chunks := prepared.chunks
	if err := s.repo.ReplaceDocumentProcessing(ctx, doc, chunks); err != nil {
		return UploadDocumentOutput{}, err
	}
	stored, err := s.repo.GetDocument(ctx, doc.ID)
	if err != nil {
		return UploadDocumentOutput{}, err
	}
	return UploadDocumentOutput{Document: stored, Chunks: chunks}, nil
}

func (s *Service) Search(ctx context.Context, access Access, input SearchInput) ([]SearchResult, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	c, err := s.GetCollection(ctx, access, strings.TrimSpace(input.CollectionID))
	if err != nil {
		return nil, err
	}
	vectors, _, err := createEmbeddings(ctx, s.provider, c.EmbeddingModel, []string{query}, s.spendGuard, access.APIKeyID)
	if err != nil {
		return nil, err
	}
	results, err := s.vectorStore.Search(ctx, c.QdrantCollection, vectors[0], input.Limit, input.Filter)
	if err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(results))
	for _, result := range results {
		payload := result.Payload
		out = append(out, SearchResult{
			DocumentID: stringFromPayload(payload, "document_id"),
			ChunkID:    stringFromPayload(payload, "chunk_id"),
			Score:      result.Score,
			Content:    stringFromPayload(payload, "content"),
			FileName:   stringFromPayload(payload, "file_name"),
			Metadata:   mapFromPayload(payload, "metadata"),
		})
	}
	return out, nil
}

func (s *Service) processNewDocument(ctx context.Context, c sqlite.RAGCollection, input UploadDocumentInput, apiKeyID int64) (UploadDocumentOutput, error) {
	input.FileName = strings.TrimSpace(filepath.Base(input.FileName))
	if input.FileName == "." || input.FileName == "" {
		return UploadDocumentOutput{}, errors.New("file_name is required")
	}
	if len(input.Bytes) == 0 {
		return UploadDocumentOutput{}, errors.New("document is empty")
	}
	if strings.TrimSpace(input.MimeType) == "" {
		input.MimeType = "application/octet-stream"
	}
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = "upload"
	}
	sourceID := strings.TrimSpace(input.SourceID)
	var existing []sqlite.RAGDocument
	if sourceID != "" {
		var err error
		existing, err = s.repo.ListDocumentsBySource(ctx, c.ID, sourceType, sourceID)
		if err != nil {
			return UploadDocumentOutput{}, err
		}
	}
	doc := sqlite.RAGDocument{
		ID:           newID("ragdoc"),
		CollectionID: c.ID,
		SourceType:   sourceType,
		SourceID:     sourceID,
		SourceURI:    strings.TrimSpace(input.SourceURI),
		FileName:     input.FileName,
		MimeType:     strings.TrimSpace(input.MimeType),
		Status:       DocumentStatusProcessing,
		Metadata:     nonNilMap(input.Metadata),
	}
	if len(existing) > 0 {
		doc.ID = existing[0].ID
	}
	failNew := func(cause error) (UploadDocumentOutput, error) {
		if len(existing) > 0 {
			return UploadDocumentOutput{}, cause
		}
		now := time.Now().UTC()
		doc.Status = DocumentStatusFailed
		doc.ErrorMessage = cause.Error()
		doc.QdrantPointCount = 0
		doc.ProcessedAt = &now
		if createErr := s.repo.CreateDocument(ctx, doc, input.Bytes, nil); createErr != nil {
			return UploadDocumentOutput{}, createErr
		}
		return UploadDocumentOutput{}, cause
	}

	prepared, err := s.prepareDocumentContent(ctx, c, &doc, input.Bytes, apiKeyID)
	if err != nil {
		return failNew(err)
	}
	if len(prepared.points) == 0 || len(prepared.points[0].Vector) == 0 {
		return failNew(errors.New("provider returned empty embedding"))
	}
	if err := s.vectorStore.EnsureCollection(ctx, c.QdrantCollection, len(prepared.points[0].Vector)); err != nil {
		return failNew(err)
	}
	if len(existing) > 0 {
		for _, old := range existing {
			if err := s.vectorStore.DeleteDocumentPoints(ctx, c.QdrantCollection, old.ID); err != nil {
				return UploadDocumentOutput{}, err
			}
		}
	}
	if err := s.vectorStore.UpsertPoints(ctx, c.QdrantCollection, prepared.points); err != nil {
		return failNew(err)
	}
	applyPreparedDocument(&doc, prepared)
	if len(existing) > 0 {
		deleteIDs := make([]string, 0, len(existing)-1)
		for _, old := range existing[1:] {
			deleteIDs = append(deleteIDs, old.ID)
		}
		if err := s.repo.ReplaceSourceDocument(ctx, doc, input.Bytes, prepared.chunks, deleteIDs); err != nil {
			_ = s.vectorStore.DeleteDocumentPoints(ctx, c.QdrantCollection, doc.ID)
			return UploadDocumentOutput{}, err
		}
	} else if err := s.repo.CreateDocument(ctx, doc, input.Bytes, prepared.chunks); err != nil {
		if sourceID != "" {
			_ = s.vectorStore.DeleteDocumentPoints(ctx, c.QdrantCollection, doc.ID)
		}
		return UploadDocumentOutput{}, err
	}
	stored, err := s.repo.GetDocument(ctx, doc.ID)
	if err != nil {
		return UploadDocumentOutput{}, err
	}
	return UploadDocumentOutput{Document: stored, Chunks: prepared.chunks}, nil
}

type preparedDocument struct {
	chunks   []sqlite.RAGDocumentChunk
	points   []QdrantPoint
	textHash string
}

func (s *Service) prepareDocumentContent(ctx context.Context, c sqlite.RAGCollection, doc *sqlite.RAGDocument, blob []byte, apiKeyID int64) (preparedDocument, error) {
	text, err := ExtractText(doc.FileName, doc.MimeType, blob)
	if err != nil {
		return preparedDocument{}, err
	}
	chunkTexts := ChunkText(text, c.ChunkSize, c.ChunkOverlap)
	if len(chunkTexts) == 0 {
		return preparedDocument{}, errors.New("document has no extractable text")
	}
	vectors, _, err := createEmbeddings(ctx, s.provider, c.EmbeddingModel, chunkTexts, s.spendGuard, apiKeyID)
	if err != nil {
		return preparedDocument{}, err
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return preparedDocument{}, errors.New("provider returned empty embedding")
	}
	chunks := make([]sqlite.RAGDocumentChunk, 0, len(chunkTexts))
	points := make([]QdrantPoint, 0, len(chunkTexts))
	for i, chunkText := range chunkTexts {
		chunkID := newID("ragchunk")
		pointID := newPointID()
		chunk := sqlite.RAGDocumentChunk{
			ID:            chunkID,
			DocumentID:    doc.ID,
			CollectionID:  c.ID,
			ChunkIndex:    i,
			Content:       chunkText,
			TokenCount:    tokenEstimate(chunkText),
			QdrantPointID: pointID,
			Metadata: map[string]any{
				"file_name": doc.FileName,
			},
		}
		chunks = append(chunks, chunk)
		points = append(points, QdrantPoint{
			ID:     pointID,
			Vector: vectors[i],
			Payload: map[string]any{
				"collection_id": c.ID,
				"document_id":   doc.ID,
				"chunk_id":      chunkID,
				"chunk_index":   i,
				"content":       chunkText,
				"file_name":     doc.FileName,
				"source_type":   doc.SourceType,
				"source_id":     doc.SourceID,
				"metadata":      doc.Metadata,
			},
		})
	}
	sum := sha256.Sum256([]byte(text))
	return preparedDocument{
		chunks:   chunks,
		points:   points,
		textHash: hex.EncodeToString(sum[:]),
	}, nil
}

func (s *Service) storePoints(ctx context.Context, collection string, points []QdrantPoint) error {
	if len(points) == 0 || len(points[0].Vector) == 0 {
		return errors.New("provider returned empty embedding")
	}
	if err := s.vectorStore.EnsureCollection(ctx, collection, len(points[0].Vector)); err != nil {
		return err
	}
	return s.vectorStore.UpsertPoints(ctx, collection, points)
}

func applyPreparedDocument(doc *sqlite.RAGDocument, prepared preparedDocument) {
	now := time.Now().UTC()
	doc.TextHash = prepared.textHash
	doc.Status = DocumentStatusReady
	doc.QdrantPointCount = len(prepared.points)
	doc.ErrorMessage = ""
	doc.ProcessedAt = &now
}

func collectionReadable(c sqlite.RAGCollection, access Access) bool {
	if access.IsAdmin {
		return true
	}
	if !c.IsActive {
		return false
	}
	if c.OwnerAPIKeyID != nil && *c.OwnerAPIKeyID == access.APIKeyID {
		return true
	}
	if boolACL(c.ACL, "public") {
		return true
	}
	for _, id := range int64ListACL(c.ACL, "api_key_ids") {
		if id == access.APIKeyID {
			return true
		}
	}
	return false
}

func collectionManageable(c sqlite.RAGCollection, access Access) bool {
	if access.IsAdmin {
		return true
	}
	return c.OwnerAPIKeyID != nil && *c.OwnerAPIKeyID == access.APIKeyID
}

func normalizeChunkSettings(chunkSize, chunkOverlap int) (int, int, error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize < 100 || chunkSize > 16000 {
		return 0, 0, errors.New("chunk_size must be between 100 and 16000")
	}
	if chunkOverlap < 0 {
		return 0, 0, errors.New("chunk_overlap must be zero or greater")
	}
	if chunkOverlap >= chunkSize {
		return 0, 0, errors.New("chunk_overlap must be smaller than chunk_size")
	}
	return chunkSize, chunkOverlap, nil
}

func safeCollectionName(id string) string {
	replacer := strings.NewReplacer("-", "_", ":", "_", "/", "_")
	return replacer.Replace(id)
}

func normalizeACL(acl map[string]any) map[string]any {
	if acl == nil {
		return map[string]any{"public": false, "api_key_ids": []int64{}}
	}
	if _, ok := acl["public"]; !ok {
		acl["public"] = false
	}
	if _, ok := acl["api_key_ids"]; !ok {
		acl["api_key_ids"] = []int64{}
	}
	return acl
}

func nonNilMap(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

func boolACL(acl map[string]any, key string) bool {
	v, ok := acl[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func int64ListACL(acl map[string]any, key string) []int64 {
	raw, ok := acl[key]
	if !ok {
		return nil
	}
	switch values := raw.(type) {
	case []int64:
		return values
	case []int:
		out := make([]int64, 0, len(values))
		for _, v := range values {
			out = append(out, int64(v))
		}
		return out
	case []float64:
		out := make([]int64, 0, len(values))
		for _, v := range values {
			out = append(out, int64(v))
		}
		return out
	case []any:
		out := make([]int64, 0, len(values))
		for _, item := range values {
			switch v := item.(type) {
			case float64:
				out = append(out, int64(v))
			case int64:
				out = append(out, v)
			case int:
				out = append(out, int64(v))
			}
		}
		return out
	default:
		return nil
	}
}

func stringFromPayload(payload map[string]any, key string) string {
	v, _ := payload[key].(string)
	return v
}

func mapFromPayload(payload map[string]any, key string) map[string]any {
	v, ok := payload[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return v
}
