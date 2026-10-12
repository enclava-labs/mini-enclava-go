CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NULL,
    password_hash TEXT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_users_active_email ON users(is_active, email);

CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id INTEGER NOT NULL,
    role_id INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY(user_id, role_id),
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);

CREATE TABLE IF NOT EXISTS dashboard_sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NULL,
    email TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    revoked_at TEXT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_dashboard_sessions_expires_at ON dashboard_sessions(expires_at);

CREATE TABLE IF NOT EXISTS provider_pricing (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    input_token_cents_per_1k REAL NOT NULL DEFAULT 0,
    output_token_cents_per_1k REAL NOT NULL DEFAULT 0,
    embedding_token_cents_per_1k REAL NOT NULL DEFAULT 0,
    effective_at TEXT NOT NULL DEFAULT (datetime('now')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(provider_id, model_id, effective_at)
);
CREATE INDEX IF NOT EXISTS idx_provider_pricing_provider_model ON provider_pricing(provider_id, model_id, effective_at DESC);

CREATE TABLE IF NOT EXISTS pricing_audit_log (
    id TEXT PRIMARY KEY,
    provider_pricing_id INTEGER NULL,
    action TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NULL,
    before_json TEXT NULL,
    after_json TEXT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(provider_pricing_id) REFERENCES provider_pricing(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_pricing_audit_log_created_at ON pricing_audit_log(created_at DESC);

CREATE TABLE IF NOT EXISTS billing_audit_log (
    id TEXT PRIMARY KEY,
    api_key_id INTEGER NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NULL,
    action TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NULL,
    amount_cents INTEGER NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_billing_audit_log_api_key_created_at ON billing_audit_log(api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_billing_audit_log_subject_created_at ON billing_audit_log(subject_type, subject_id, created_at DESC);

CREATE TABLE IF NOT EXISTS budgets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NULL,
    api_key_id INTEGER NULL,
    limit_tokens INTEGER NULL,
    limit_cents INTEGER NULL,
    period TEXT NOT NULL DEFAULT 'monthly',
    starts_at TEXT NULL,
    ends_at TEXT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(api_key_id) REFERENCES api_keys(id) ON DELETE CASCADE,
    CHECK (limit_tokens IS NOT NULL OR limit_cents IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_budgets_api_key_active ON budgets(api_key_id, is_active);
CREATE INDEX IF NOT EXISTS idx_budgets_subject_active ON budgets(subject_type, subject_id, is_active);

CREATE TABLE IF NOT EXISTS rag_collections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NULL,
    owner_api_key_id INTEGER NULL,
    qdrant_collection TEXT NOT NULL UNIQUE,
    embedding_provider TEXT NULL,
    embedding_model TEXT NOT NULL,
    chunk_size INTEGER NOT NULL DEFAULT 1000,
    chunk_overlap INTEGER NOT NULL DEFAULT 150,
    acl_json TEXT NOT NULL DEFAULT '{}',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(owner_api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_rag_collections_owner ON rag_collections(owner_api_key_id, is_active);
CREATE INDEX IF NOT EXISTS idx_rag_collections_name ON rag_collections(name);

CREATE TABLE IF NOT EXISTS rag_documents (
    id TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NULL,
    source_uri TEXT NULL,
    file_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    status TEXT NOT NULL,
    text_hash TEXT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    qdrant_point_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at TEXT NULL,
    FOREIGN KEY(collection_id) REFERENCES rag_collections(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_rag_documents_collection_status ON rag_documents(collection_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rag_documents_source ON rag_documents(source_type, source_id);

CREATE TABLE IF NOT EXISTS rag_document_chunks (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    qdrant_point_id TEXT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(document_id) REFERENCES rag_documents(id) ON DELETE CASCADE,
    FOREIGN KEY(collection_id) REFERENCES rag_collections(id) ON DELETE CASCADE,
    UNIQUE(document_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_rag_document_chunks_collection ON rag_document_chunks(collection_id);
CREATE INDEX IF NOT EXISTS idx_rag_document_chunks_point ON rag_document_chunks(qdrant_point_id);

CREATE TABLE IF NOT EXISTS prompt_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NULL,
    system_prompt TEXT NOT NULL,
    user_prompt TEXT NOT NULL,
    variables_schema TEXT NOT NULL DEFAULT '{}',
    is_default INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_active_name ON prompt_templates(is_active, name);

CREATE TABLE IF NOT EXISTS chatbot_instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NULL,
    prompt_template_id TEXT NULL,
    provider_id TEXT NULL,
    model TEXT NOT NULL,
    rag_collection_ids TEXT NOT NULL DEFAULT '[]',
    settings_json TEXT NOT NULL DEFAULT '{}',
    external_id TEXT NOT NULL UNIQUE,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(prompt_template_id) REFERENCES prompt_templates(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_chatbot_instances_enabled_name ON chatbot_instances(is_enabled, name);

CREATE TABLE IF NOT EXISTS chatbot_conversations (
    id TEXT PRIMARY KEY,
    chatbot_id TEXT NOT NULL,
    api_key_id INTEGER NULL,
    external_user_id TEXT NULL,
    title TEXT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(chatbot_id) REFERENCES chatbot_instances(id) ON DELETE CASCADE,
    FOREIGN KEY(api_key_id) REFERENCES api_keys(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_chatbot_conversations_chatbot_created ON chatbot_conversations(chatbot_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chatbot_conversations_api_key_created ON chatbot_conversations(api_key_id, created_at DESC);

CREATE TABLE IF NOT EXISTS chatbot_messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    model TEXT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(conversation_id) REFERENCES chatbot_conversations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chatbot_messages_conversation_created ON chatbot_messages(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS connector_sources (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    target_collection_id TEXT NULL,
    config_json TEXT NOT NULL DEFAULT '{}',
    credential_ref TEXT NULL,
    checkpoint_json TEXT NOT NULL DEFAULT '{}',
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_sync_at TEXT NULL,
    FOREIGN KEY(target_collection_id) REFERENCES rag_collections(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_connector_sources_type_enabled ON connector_sources(type, is_enabled);
CREATE INDEX IF NOT EXISTS idx_connector_sources_target_collection ON connector_sources(target_collection_id);

CREATE TABLE IF NOT EXISTS connector_sync_jobs (
    id TEXT PRIMARY KEY,
    connector_source_id TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NULL,
    completed_at TEXT NULL,
    error_message TEXT NULL,
    stats_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY(connector_source_id) REFERENCES connector_sources(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_connector_sync_jobs_source_created ON connector_sync_jobs(connector_source_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_connector_sync_jobs_status_created ON connector_sync_jobs(status, created_at DESC);
