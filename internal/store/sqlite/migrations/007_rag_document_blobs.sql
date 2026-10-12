CREATE TABLE IF NOT EXISTS rag_document_blobs (
    document_id TEXT PRIMARY KEY,
    content BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(document_id) REFERENCES rag_documents(id) ON DELETE CASCADE
);
