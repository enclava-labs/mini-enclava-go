package rag

import "enclava-go/internal/store/sqlite"

const (
	DocumentStatusProcessing = "processing"
	DocumentStatusReady      = "ready"
	DocumentStatusFailed     = "failed"

	DefaultChunkSize    = 1000
	DefaultChunkOverlap = 150
)

type CreateCollectionInput struct {
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	EmbeddingModel string         `json:"embedding_model,omitempty"`
	ChunkSize      int            `json:"chunk_size,omitempty"`
	ChunkOverlap   int            `json:"chunk_overlap,omitempty"`
	ACL            map[string]any `json:"acl,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type UpdateCollectionInput struct {
	Name           *string        `json:"name,omitempty"`
	Description    *string        `json:"description,omitempty"`
	EmbeddingModel *string        `json:"embedding_model,omitempty"`
	ChunkSize      *int           `json:"chunk_size,omitempty"`
	ChunkOverlap   *int           `json:"chunk_overlap,omitempty"`
	ACL            map[string]any `json:"acl,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	IsActive       *bool          `json:"is_active,omitempty"`
}

type UploadDocumentInput struct {
	FileName   string         `json:"file_name"`
	MimeType   string         `json:"mime_type"`
	Bytes      []byte         `json:"-"`
	SourceType string         `json:"source_type,omitempty"`
	SourceID   string         `json:"source_id,omitempty"`
	SourceURI  string         `json:"source_uri,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type SearchInput struct {
	CollectionID string         `json:"collection_id"`
	Query        string         `json:"query"`
	Limit        int            `json:"limit,omitempty"`
	Filter       map[string]any `json:"filter,omitempty"`
}

type SearchResult struct {
	DocumentID string         `json:"document_id"`
	ChunkID    string         `json:"chunk_id"`
	Score      float64        `json:"score"`
	Content    string         `json:"content"`
	FileName   string         `json:"file_name,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type UploadDocumentOutput struct {
	Document sqlite.RAGDocument        `json:"document"`
	Chunks   []sqlite.RAGDocumentChunk `json:"chunks"`
}
