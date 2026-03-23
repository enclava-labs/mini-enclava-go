package extract

import "enclava-go/internal/store/sqlite"

type ProcessInput struct {
	TemplateID string
	Context    map[string]any
	FileName   string
	MimeType   string
	Bytes      []byte
}

type ProcessOutput struct {
	Success              bool           `json:"success"`
	JobID                string         `json:"job_id"`
	ModelUsed            string         `json:"model_used,omitempty"`
	Data                 map[string]any `json:"data"`
	RawResponse          string         `json:"raw_response"`
	ValidationErrors     []string       `json:"validation_errors"`
	ValidationWarnings   []string       `json:"validation_warnings"`
	ProcessingTimeMS     int64          `json:"processing_time_ms"`
	PromptTokensUsed     int            `json:"prompt_tokens_used"`
	CompletionTokensUsed int            `json:"completion_tokens_used"`
	TokensUsed           int            `json:"tokens_used"`
	CostCents            int            `json:"cost_cents"`
}

type JobDetail struct {
	Job    sqlite.ExtractJob     `json:"job"`
	Result *sqlite.ExtractResult `json:"result,omitempty"`
}

const (
	StatusPending             = "pending"
	StatusProcessing          = "processing"
	StatusCompleted           = "completed"
	StatusCompletedWithErrors = "completed_with_errors"
	StatusFailed              = "failed"
)
