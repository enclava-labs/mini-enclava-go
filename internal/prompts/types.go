package prompts

type CreateTemplateInput struct {
	ID              string         `json:"id,omitempty"`
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	SystemPrompt    string         `json:"system_prompt"`
	UserPrompt      string         `json:"user_prompt"`
	VariablesSchema map[string]any `json:"variables_schema,omitempty"`
	IsDefault       bool           `json:"is_default,omitempty"`
	IsActive        *bool          `json:"is_active,omitempty"`
}

type UpdateTemplateInput struct {
	Name            *string        `json:"name,omitempty"`
	Description     *string        `json:"description,omitempty"`
	SystemPrompt    *string        `json:"system_prompt,omitempty"`
	UserPrompt      *string        `json:"user_prompt,omitempty"`
	VariablesSchema map[string]any `json:"variables_schema,omitempty"`
	IsDefault       *bool          `json:"is_default,omitempty"`
	IsActive        *bool          `json:"is_active,omitempty"`
}

type RenderInput struct {
	UserInput  string
	RAGContext string
	Variables  map[string]any
}

type RenderedPrompt struct {
	SystemPrompt string
	UserPrompt   string
}
