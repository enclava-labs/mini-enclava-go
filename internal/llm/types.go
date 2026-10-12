package llm

import (
	"encoding/json"
	"strings"
)

type Model struct {
	ID                      string         `json:"id"`
	Object                  string         `json:"object,omitempty"`
	OwnedBy                 string         `json:"owned_by,omitempty"`
	InputModalities         []string       `json:"input_modalities,omitempty"`
	OutputModalities        []string       `json:"output_modalities,omitempty"`
	Tasks                   []string       `json:"tasks,omitempty"`
	ContextLength           int64          `json:"context_length,omitempty"`
	MaxOutputTokens         int64          `json:"max_output_tokens,omitempty"`
	SupportsStreaming       bool           `json:"supports_streaming,omitempty"`
	SupportsFunctionCalling bool           `json:"supports_function_calling,omitempty"`
	SupportsVision          bool           `json:"supports_vision,omitempty"`
	SupportsDocuments       bool           `json:"supports_documents,omitempty"`
	SupportsEmbeddings      bool           `json:"supports_embeddings,omitempty"`
	Provider                string         `json:"provider,omitempty"`
	Metadata                map[string]any `json:"metadata,omitempty"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type ChatRequest struct {
	Model  string          `json:"model"`
	Stream bool            `json:"stream,omitempty"`
	Raw    json.RawMessage `json:"-"`
}

func SupportsVisionOrDocument(m Model) bool {
	if m.SupportsVision || m.SupportsDocuments {
		return true
	}
	for _, modality := range m.InputModalities {
		switch strings.ToLower(modality) {
		case "image", "vision", "pdf", "document", "file":
			return true
		}
	}
	for _, task := range m.Tasks {
		switch strings.ToLower(task) {
		case "vision", "pdf", "document", "file":
			return true
		}
	}
	return false
}

func NormalizeModels(resp ModelsResponse, provider string) ModelsResponse {
	if resp.Object == "" {
		resp.Object = "list"
	}
	for i := range resp.Data {
		resp.Data[i] = NormalizeModel(resp.Data[i], provider)
	}
	return resp
}

func NormalizeModel(m Model, provider string) Model {
	if m.Object == "" {
		m.Object = "model"
	}
	if m.Provider == "" {
		m.Provider = provider
	}
	if m.OwnedBy == "" && provider != "" {
		m.OwnedBy = provider
	}
	id := strings.ToLower(m.ID)
	for _, modality := range m.InputModalities {
		switch strings.ToLower(modality) {
		case "image", "vision":
			m.SupportsVision = true
		case "pdf", "document", "file":
			m.SupportsDocuments = true
		}
	}
	for _, task := range m.Tasks {
		switch strings.ToLower(task) {
		case "vision":
			m.SupportsVision = true
		case "pdf", "document", "file":
			m.SupportsDocuments = true
		case "embedding", "embeddings":
			m.SupportsEmbeddings = true
		case "chat", "chat.completions":
			m.SupportsStreaming = true
		}
	}
	if strings.Contains(id, "embed") {
		m.SupportsEmbeddings = true
	}
	if strings.Contains(id, "vision") || strings.Contains(id, "vl") {
		m.SupportsVision = true
	}
	if strings.Contains(id, "gpt-4o") || strings.Contains(id, "o4") {
		m.SupportsVision = true
		m.SupportsDocuments = true
	}
	if !m.SupportsEmbeddings {
		m.SupportsStreaming = true
	}
	return m
}
