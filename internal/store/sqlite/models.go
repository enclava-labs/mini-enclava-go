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
	AllowedIPs              []string
	RateLimitPerMinute      *int
	RateLimitPerHour        *int
	RateLimitPerDay         *int
	IsUnlimited             bool
	BudgetLimitTokens       *int64
	BudgetLimitCents        *int64
	BudgetPeriod            string
	Tags                    []string

	TotalRequests  int64
	TotalTokens    int64
	TotalCostCents int64
	LastUsedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ExtractTemplate struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	SystemPrompt  string         `json:"system_prompt"`
	UserPrompt    string         `json:"user_prompt"`
	ContextSchema map[string]any `json:"context_schema,omitempty"`
	Model         string         `json:"model,omitempty"`
	IsDefault     bool           `json:"is_default"`
	IsActive      bool           `json:"is_active"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
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
