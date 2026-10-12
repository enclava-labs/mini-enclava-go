package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type RAGRepo struct {
	db *sql.DB
}

func NewRAGRepo(db *sql.DB) *RAGRepo {
	return &RAGRepo{db: db}
}

func (r *RAGRepo) CreateCollection(ctx context.Context, c RAGCollection) error {
	aclJSON, err := marshalJSON(c.ACL)
	if err != nil {
		return fmt.Errorf("marshal collection acl: %w", err)
	}
	metadataJSON, err := marshalJSON(c.Metadata)
	if err != nil {
		return fmt.Errorf("marshal collection metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO rag_collections(
			id, name, description, owner_api_key_id, qdrant_collection,
			embedding_provider, embedding_model, chunk_size, chunk_overlap,
			acl_json, metadata_json, is_active
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.Name, nullableString(c.Description), nullableInt64(c.OwnerAPIKeyID), c.QdrantCollection,
		nullableString(c.EmbeddingProvider), c.EmbeddingModel, c.ChunkSize, c.ChunkOverlap,
		aclJSON, metadataJSON, boolToInt(c.IsActive))
	if err != nil {
		return fmt.Errorf("create rag collection: %w", err)
	}
	return nil
}

func (r *RAGRepo) ListCollections(ctx context.Context) ([]RAGCollection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, owner_api_key_id, qdrant_collection,
		       embedding_provider, embedding_model, chunk_size, chunk_overlap,
		       acl_json, metadata_json, is_active, created_at, updated_at
		FROM rag_collections
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list rag collections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []RAGCollection{}
	for rows.Next() {
		c, err := scanRAGCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *RAGRepo) GetCollection(ctx context.Context, id string) (RAGCollection, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, owner_api_key_id, qdrant_collection,
		       embedding_provider, embedding_model, chunk_size, chunk_overlap,
		       acl_json, metadata_json, is_active, created_at, updated_at
		FROM rag_collections
		WHERE id = ?
	`, id)
	return scanRAGCollection(row)
}

func (r *RAGRepo) UpdateCollection(ctx context.Context, c RAGCollection) error {
	aclJSON, err := marshalJSON(c.ACL)
	if err != nil {
		return fmt.Errorf("marshal collection acl: %w", err)
	}
	metadataJSON, err := marshalJSON(c.Metadata)
	if err != nil {
		return fmt.Errorf("marshal collection metadata: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE rag_collections
		SET name = ?,
		    description = ?,
		    embedding_provider = ?,
		    embedding_model = ?,
		    chunk_size = ?,
		    chunk_overlap = ?,
		    acl_json = ?,
		    metadata_json = ?,
		    is_active = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, c.Name, nullableString(c.Description), nullableString(c.EmbeddingProvider),
		c.EmbeddingModel, c.ChunkSize, c.ChunkOverlap, aclJSON, metadataJSON,
		boolToInt(c.IsActive), c.ID)
	if err != nil {
		return fmt.Errorf("update rag collection: %w", err)
	}
	return nil
}

func (r *RAGRepo) DeleteCollection(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM rag_collections WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete rag collection: %w", err)
	}
	return nil
}

func (r *RAGRepo) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]RAGDocument, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, collection_id, source_type, source_id, source_uri, file_name,
		       mime_type, status, text_hash, metadata_json, qdrant_point_count,
		       error_message, created_at, updated_at, processed_at
		FROM rag_documents
		WHERE collection_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, collectionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list rag documents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []RAGDocument{}
	for rows.Next() {
		doc, err := scanRAGDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (r *RAGRepo) ListDocumentsBySource(ctx context.Context, collectionID, sourceType, sourceID string) ([]RAGDocument, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, collection_id, source_type, source_id, source_uri, file_name,
		       mime_type, status, text_hash, metadata_json, qdrant_point_count,
		       error_message, created_at, updated_at, processed_at
		FROM rag_documents
		WHERE collection_id = ? AND source_type = ? AND source_id = ?
		ORDER BY created_at ASC, rowid ASC
	`, collectionID, sourceType, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list rag documents by source: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []RAGDocument{}
	for rows.Next() {
		doc, err := scanRAGDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (r *RAGRepo) GetDocument(ctx context.Context, id string) (RAGDocument, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, collection_id, source_type, source_id, source_uri, file_name,
		       mime_type, status, text_hash, metadata_json, qdrant_point_count,
		       error_message, created_at, updated_at, processed_at
		FROM rag_documents
		WHERE id = ?
	`, id)
	return scanRAGDocument(row)
}

func (r *RAGRepo) CreateDocument(ctx context.Context, doc RAGDocument, blob []byte, chunks []RAGDocumentChunk) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rag document tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	metadataJSON, err := marshalJSON(doc.Metadata)
	if err != nil {
		return fmt.Errorf("marshal document metadata: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO rag_documents(
			id, collection_id, source_type, source_id, source_uri, file_name,
			mime_type, status, text_hash, metadata_json, qdrant_point_count,
			error_message, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, doc.ID, doc.CollectionID, doc.SourceType, nullableString(doc.SourceID), nullableString(doc.SourceURI),
		doc.FileName, doc.MimeType, doc.Status, nullableString(doc.TextHash), metadataJSON,
		doc.QdrantPointCount, nullableString(doc.ErrorMessage), nullableTime(doc.ProcessedAt))
	if err != nil {
		return fmt.Errorf("insert rag document: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rag_document_blobs(document_id, content) VALUES (?, ?)
	`, doc.ID, blob); err != nil {
		return fmt.Errorf("insert rag document blob: %w", err)
	}
	for _, chunk := range chunks {
		chunkMetadataJSON, err := marshalJSON(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal chunk metadata: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rag_document_chunks(
				id, document_id, collection_id, chunk_index, content,
				token_count, qdrant_point_id, metadata_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, chunk.ID, chunk.DocumentID, chunk.CollectionID, chunk.ChunkIndex, chunk.Content,
			chunk.TokenCount, nullableString(chunk.QdrantPointID), chunkMetadataJSON); err != nil {
			return fmt.Errorf("insert rag document chunk: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rag document tx: %w", err)
	}
	return nil
}

func (r *RAGRepo) ReplaceSourceDocument(ctx context.Context, doc RAGDocument, blob []byte, chunks []RAGDocumentChunk, deleteIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rag source replace tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	metadataJSON, err := marshalJSON(doc.Metadata)
	if err != nil {
		return fmt.Errorf("marshal document metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE rag_documents
		SET source_uri = ?,
		    file_name = ?,
		    mime_type = ?,
		    status = ?,
		    text_hash = ?,
		    metadata_json = ?,
		    qdrant_point_count = ?,
		    error_message = ?,
		    processed_at = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, nullableString(doc.SourceURI), doc.FileName, doc.MimeType, doc.Status, nullableString(doc.TextHash),
		metadataJSON, doc.QdrantPointCount, nullableString(doc.ErrorMessage), nullableTime(doc.ProcessedAt), doc.ID); err != nil {
		return fmt.Errorf("update rag source document: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rag_document_blobs(document_id, content) VALUES (?, ?)
		ON CONFLICT(document_id) DO UPDATE SET content = excluded.content
	`, doc.ID, blob); err != nil {
		return fmt.Errorf("replace rag document blob: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM rag_document_chunks WHERE document_id = ?", doc.ID); err != nil {
		return fmt.Errorf("delete old rag chunks: %w", err)
	}
	for _, chunk := range chunks {
		chunkMetadataJSON, err := marshalJSON(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal chunk metadata: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rag_document_chunks(
				id, document_id, collection_id, chunk_index, content,
				token_count, qdrant_point_id, metadata_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, chunk.ID, chunk.DocumentID, chunk.CollectionID, chunk.ChunkIndex, chunk.Content,
			chunk.TokenCount, nullableString(chunk.QdrantPointID), chunkMetadataJSON); err != nil {
			return fmt.Errorf("insert replacement rag chunk: %w", err)
		}
	}
	for _, id := range deleteIDs {
		if id == "" || id == doc.ID {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM rag_documents WHERE id = ?", id); err != nil {
			return fmt.Errorf("delete duplicate rag document: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rag source replace tx: %w", err)
	}
	return nil
}

func (r *RAGRepo) ReplaceDocumentProcessing(ctx context.Context, doc RAGDocument, chunks []RAGDocumentChunk) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rag reprocess tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	metadataJSON, err := marshalJSON(doc.Metadata)
	if err != nil {
		return fmt.Errorf("marshal document metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE rag_documents
		SET status = ?,
		    text_hash = ?,
		    metadata_json = ?,
		    qdrant_point_count = ?,
		    error_message = ?,
		    processed_at = ?,
		    updated_at = datetime('now')
		WHERE id = ?
	`, doc.Status, nullableString(doc.TextHash), metadataJSON, doc.QdrantPointCount,
		nullableString(doc.ErrorMessage), nullableTime(doc.ProcessedAt), doc.ID); err != nil {
		return fmt.Errorf("update rag document processing: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM rag_document_chunks WHERE document_id = ?", doc.ID); err != nil {
		return fmt.Errorf("delete old rag chunks: %w", err)
	}
	for _, chunk := range chunks {
		chunkMetadataJSON, err := marshalJSON(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("marshal chunk metadata: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rag_document_chunks(
				id, document_id, collection_id, chunk_index, content,
				token_count, qdrant_point_id, metadata_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, chunk.ID, chunk.DocumentID, chunk.CollectionID, chunk.ChunkIndex, chunk.Content,
			chunk.TokenCount, nullableString(chunk.QdrantPointID), chunkMetadataJSON); err != nil {
			return fmt.Errorf("insert replacement rag chunk: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rag reprocess tx: %w", err)
	}
	return nil
}

func (r *RAGRepo) GetDocumentBlob(ctx context.Context, documentID string) ([]byte, error) {
	var blob []byte
	if err := r.db.QueryRowContext(ctx, `
		SELECT content FROM rag_document_blobs WHERE document_id = ?
	`, documentID).Scan(&blob); err != nil {
		return nil, fmt.Errorf("get rag document blob: %w", err)
	}
	return blob, nil
}

func (r *RAGRepo) ListDocumentChunks(ctx context.Context, documentID string) ([]RAGDocumentChunk, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, document_id, collection_id, chunk_index, content,
		       token_count, qdrant_point_id, metadata_json, created_at
		FROM rag_document_chunks
		WHERE document_id = ?
		ORDER BY chunk_index
	`, documentID)
	if err != nil {
		return nil, fmt.Errorf("list rag document chunks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []RAGDocumentChunk{}
	for rows.Next() {
		chunk, err := scanRAGDocumentChunk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	return out, rows.Err()
}

func (r *RAGRepo) DeleteDocument(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM rag_documents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete rag document: %w", err)
	}
	return nil
}

type ragCollectionScanner interface {
	Scan(dest ...any) error
}

func scanRAGCollection(s ragCollectionScanner) (RAGCollection, error) {
	var c RAGCollection
	var desc sql.NullString
	var owner sql.NullInt64
	var provider sql.NullString
	var aclRaw, metadataRaw string
	var isActive int
	var createdRaw, updatedRaw string
	if err := s.Scan(
		&c.ID, &c.Name, &desc, &owner, &c.QdrantCollection,
		&provider, &c.EmbeddingModel, &c.ChunkSize, &c.ChunkOverlap,
		&aclRaw, &metadataRaw, &isActive, &createdRaw, &updatedRaw,
	); err != nil {
		return RAGCollection{}, fmt.Errorf("scan rag collection: %w", err)
	}
	if desc.Valid {
		c.Description = desc.String
	}
	if owner.Valid {
		v := owner.Int64
		c.OwnerAPIKeyID = &v
	}
	if provider.Valid {
		c.EmbeddingProvider = provider.String
	}
	c.ACL = unmarshalMap(aclRaw)
	c.Metadata = unmarshalMap(metadataRaw)
	c.IsActive = isActive == 1
	created, err := parseDBTime(createdRaw)
	if err != nil {
		return RAGCollection{}, err
	}
	updated, err := parseDBTime(updatedRaw)
	if err != nil {
		return RAGCollection{}, err
	}
	c.CreatedAt = created
	c.UpdatedAt = updated
	return c, nil
}

type ragDocumentScanner interface {
	Scan(dest ...any) error
}

func scanRAGDocument(s ragDocumentScanner) (RAGDocument, error) {
	var d RAGDocument
	var sourceID, sourceURI, textHash, errorMessage, processedAt sql.NullString
	var metadataRaw string
	var createdRaw, updatedRaw string
	if err := s.Scan(
		&d.ID, &d.CollectionID, &d.SourceType, &sourceID, &sourceURI, &d.FileName,
		&d.MimeType, &d.Status, &textHash, &metadataRaw, &d.QdrantPointCount,
		&errorMessage, &createdRaw, &updatedRaw, &processedAt,
	); err != nil {
		return RAGDocument{}, fmt.Errorf("scan rag document: %w", err)
	}
	if sourceID.Valid {
		d.SourceID = sourceID.String
	}
	if sourceURI.Valid {
		d.SourceURI = sourceURI.String
	}
	if textHash.Valid {
		d.TextHash = textHash.String
	}
	if errorMessage.Valid {
		d.ErrorMessage = errorMessage.String
	}
	d.Metadata = unmarshalMap(metadataRaw)
	created, err := parseDBTime(createdRaw)
	if err != nil {
		return RAGDocument{}, err
	}
	updated, err := parseDBTime(updatedRaw)
	if err != nil {
		return RAGDocument{}, err
	}
	d.CreatedAt = created
	d.UpdatedAt = updated
	if processedAt.Valid && processedAt.String != "" {
		t, err := parseDBTime(processedAt.String)
		if err == nil {
			d.ProcessedAt = &t
		}
	}
	return d, nil
}

type ragDocumentChunkScanner interface {
	Scan(dest ...any) error
}

func scanRAGDocumentChunk(s ragDocumentChunkScanner) (RAGDocumentChunk, error) {
	var c RAGDocumentChunk
	var pointID sql.NullString
	var metadataRaw, createdRaw string
	if err := s.Scan(
		&c.ID, &c.DocumentID, &c.CollectionID, &c.ChunkIndex, &c.Content,
		&c.TokenCount, &pointID, &metadataRaw, &createdRaw,
	); err != nil {
		return RAGDocumentChunk{}, fmt.Errorf("scan rag document chunk: %w", err)
	}
	if pointID.Valid {
		c.QdrantPointID = pointID.String
	}
	c.Metadata = unmarshalMap(metadataRaw)
	created, err := parseDBTime(createdRaw)
	if err != nil {
		return RAGDocumentChunk{}, err
	}
	c.CreatedAt = created
	return c, nil
}
