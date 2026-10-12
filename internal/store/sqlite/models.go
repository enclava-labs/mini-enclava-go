package sqlite

import "time"

type APIKey struct {
	ID                      int64
	Name                    string
	Description             string
	KeyPrefix               string
	KeyHash                 string
	KeyHashAlgo             string
	IsActive                bool
	IsDeleted               bool
	DeletedAt               *time.Time
	ExpiresAt               *time.Time
	Scopes                  []string
	AllowedModels           []string
	AllowedEndpoints        []string
	AllowedExtractTemplates []string
	AllowedChatbots         []string
	AllowedAgents           []string
	AllowedIPs              []string
	RateLimitPerMinute      *int
	RateLimitPerHour        *int
	RateLimitPerDay         *int
	IsUnlimited             bool
	BudgetLimitTokens       *int64
	BudgetLimitCents        *int64
	BudgetPeriod            string
	Tags                    []string

	TotalRequests       int64
	TotalTokens         int64
	TotalCostCents      int64
	TotalCostMicrocents int64 `json:"-"`
	LastUsedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ExtractTemplate struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	SystemPrompt  string         `json:"system_prompt"`
	UserPrompt    string         `json:"user_prompt"`
	ContextSchema map[string]any `json:"context_schema,omitempty"`
	OutputSchema  map[string]any `json:"output_schema,omitempty"`
	Model         string         `json:"model,omitempty"`
	IsDefault     bool           `json:"is_default"`
	IsActive      bool           `json:"is_active"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type Budget struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	SubjectType string     `json:"subject_type"`
	SubjectID   string     `json:"subject_id,omitempty"`
	APIKeyID    *int64     `json:"api_key_id,omitempty"`
	LimitTokens *int64     `json:"limit_tokens,omitempty"`
	LimitCents  *int64     `json:"limit_cents,omitempty"`
	Period      string     `json:"period"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ExtractSettings struct {
	ID            int       `json:"id"`
	DefaultModel  string    `json:"default_model,omitempty"`
	MaxFileSizeMB int64     `json:"max_file_size_mb"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ExtractJob struct {
	ID            string     `json:"id"`
	APIKeyID      int64      `json:"api_key_id"`
	TemplateID    string     `json:"template_id"`
	Status        string     `json:"status"`
	FileName      string     `json:"file_name"`
	FileMimeType  string     `json:"file_mime_type"`
	FileSizeBytes int64      `json:"file_size_bytes"`
	NumPages      int        `json:"num_pages"`
	ModelUsed     string     `json:"model_used,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

type ExtractResult struct {
	ID                 string         `json:"id"`
	JobID              string         `json:"job_id"`
	Data               map[string]any `json:"data"`
	RawResponse        string         `json:"raw_response"`
	ValidationErrors   []string       `json:"validation_errors"`
	ValidationWarnings []string       `json:"validation_warnings"`
	TokensUsed         int            `json:"tokens_used"`
	CostCents          int            `json:"cost_cents"`
	CreatedAt          time.Time      `json:"created_at"`
}

type RAGCollection struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Description       string         `json:"description,omitempty"`
	OwnerAPIKeyID     *int64         `json:"owner_api_key_id,omitempty"`
	QdrantCollection  string         `json:"qdrant_collection"`
	EmbeddingProvider string         `json:"embedding_provider,omitempty"`
	EmbeddingModel    string         `json:"embedding_model"`
	ChunkSize         int            `json:"chunk_size"`
	ChunkOverlap      int            `json:"chunk_overlap"`
	ACL               map[string]any `json:"acl,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	IsActive          bool           `json:"is_active"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type RAGDocument struct {
	ID               string         `json:"id"`
	CollectionID     string         `json:"collection_id"`
	SourceType       string         `json:"source_type"`
	SourceID         string         `json:"source_id,omitempty"`
	SourceURI        string         `json:"source_uri,omitempty"`
	FileName         string         `json:"file_name"`
	MimeType         string         `json:"mime_type"`
	Status           string         `json:"status"`
	TextHash         string         `json:"text_hash,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	QdrantPointCount int            `json:"qdrant_point_count"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	ProcessedAt      *time.Time     `json:"processed_at,omitempty"`
}

type RAGDocumentChunk struct {
	ID            string         `json:"id"`
	DocumentID    string         `json:"document_id"`
	CollectionID  string         `json:"collection_id"`
	ChunkIndex    int            `json:"chunk_index"`
	Content       string         `json:"content"`
	TokenCount    int            `json:"token_count"`
	QdrantPointID string         `json:"qdrant_point_id,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

type PromptTemplate struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	SystemPrompt    string         `json:"system_prompt"`
	UserPrompt      string         `json:"user_prompt"`
	VariablesSchema map[string]any `json:"variables_schema,omitempty"`
	IsDefault       bool           `json:"is_default"`
	IsActive        bool           `json:"is_active"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type ChatbotInstance struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	PromptTemplateID string         `json:"prompt_template_id,omitempty"`
	ProviderID       string         `json:"provider_id,omitempty"`
	Model            string         `json:"model"`
	RAGCollectionIDs []string       `json:"rag_collection_ids,omitempty"`
	Settings         map[string]any `json:"settings,omitempty"`
	ExternalID       string         `json:"external_id"`
	IsEnabled        bool           `json:"is_enabled"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type ChatbotConversation struct {
	ID             string         `json:"id"`
	ChatbotID      string         `json:"chatbot_id"`
	APIKeyID       *int64         `json:"api_key_id,omitempty"`
	ExternalUserID string         `json:"external_user_id,omitempty"`
	Title          string         `json:"title,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type ChatbotMessage struct {
	ID               string         `json:"id"`
	ConversationID   string         `json:"conversation_id"`
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	Model            string         `json:"model,omitempty"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type ProviderPricing struct {
	ID                       int64     `json:"id"`
	ProviderID               string    `json:"provider_id"`
	ModelID                  string    `json:"model_id"`
	Currency                 string    `json:"currency"`
	InputTokenCentsPer1K     float64   `json:"input_token_cents_per_1k"`
	OutputTokenCentsPer1K    float64   `json:"output_token_cents_per_1k"`
	EmbeddingTokenCentsPer1K float64   `json:"embedding_token_cents_per_1k"`
	EffectiveAt              time.Time `json:"effective_at"`
	CreatedAt                time.Time `json:"created_at"`
}

type PricingAuditLog struct {
	ID                string         `json:"id"`
	ProviderPricingID *int64         `json:"provider_pricing_id,omitempty"`
	Action            string         `json:"action"`
	ActorType         string         `json:"actor_type"`
	ActorID           string         `json:"actor_id,omitempty"`
	Before            map[string]any `json:"before,omitempty"`
	After             map[string]any `json:"after,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

type BillingAuditLog struct {
	ID          string         `json:"id"`
	APIKeyID    *int64         `json:"api_key_id,omitempty"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id,omitempty"`
	Action      string         `json:"action"`
	ActorType   string         `json:"actor_type"`
	ActorID     string         `json:"actor_id,omitempty"`
	AmountCents *int64         `json:"amount_cents,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

type ProviderConfig struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Name          string         `json:"name"`
	BaseURL       string         `json:"base_url,omitempty"`
	CredentialRef string         `json:"credential_ref,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
	IsEnabled     bool           `json:"is_enabled"`
	IsDefault     bool           `json:"is_default"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type ConnectorSource struct {
	ID                 string         `json:"id"`
	Type               string         `json:"type"`
	Name               string         `json:"name"`
	TargetCollectionID string         `json:"target_collection_id,omitempty"`
	Config             map[string]any `json:"config,omitempty"`
	CredentialRef      string         `json:"credential_ref,omitempty"`
	Checkpoint         map[string]any `json:"checkpoint,omitempty"`
	IsEnabled          bool           `json:"is_enabled"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	LastSyncAt         *time.Time     `json:"last_sync_at,omitempty"`
}

type ConnectorSyncJob struct {
	ID                string         `json:"id"`
	ConnectorSourceID string         `json:"connector_source_id"`
	Status            string         `json:"status"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	ErrorMessage      string         `json:"error_message,omitempty"`
	Stats             map[string]any `json:"stats,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}
