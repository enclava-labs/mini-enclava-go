package connectors

const (
	SourceTypeGitHub = "github"
	SourceTypeSlack  = "slack"
	SourceTypeNotion = "notion"
	SourceTypeLinear = "linear"

	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
)

type Access struct {
	APIKeyID int64
	IsAdmin  bool
}

type CreateSourceInput struct {
	Type               string         `json:"type"`
	Name               string         `json:"name"`
	TargetCollectionID string         `json:"target_collection_id,omitempty"`
	Config             map[string]any `json:"config,omitempty"`
	CredentialRef      string         `json:"credential_ref,omitempty"`
	IsEnabled          *bool          `json:"is_enabled,omitempty"`
}

type UpdateSourceInput struct {
	Type               *string        `json:"type,omitempty"`
	Name               *string        `json:"name,omitempty"`
	TargetCollectionID *string        `json:"target_collection_id,omitempty"`
	Config             map[string]any `json:"config,omitempty"`
	CredentialRef      *string        `json:"credential_ref,omitempty"`
	Checkpoint         map[string]any `json:"checkpoint,omitempty"`
	IsEnabled          *bool          `json:"is_enabled,omitempty"`
}
