package llm

import (
	"encoding/json"
	"strings"
)

type Model struct {
	ID               string   `json:"id"`
	Object           string   `json:"object,omitempty"`
	OwnedBy          string   `json:"owned_by,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	Tasks            []string `json:"tasks,omitempty"`
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
