package prompts

import (
	"strings"
	"testing"

	"enclava-go/internal/store/sqlite"
)

func TestRenderInjectsPromptVariables(t *testing.T) {
	rendered, err := Render(sqlite.PromptTemplate{
		ID:           "test",
		SystemPrompt: "Answer for {{.audience}} using {{.rag_context}}",
		UserPrompt:   "Question: {{.user_input}}",
	}, RenderInput{
		UserInput:  "Where is the citadel?",
		RAGContext: "maple-vector-citadel",
		Variables: map[string]any{
			"audience": "operators",
		},
	})
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if !strings.Contains(rendered.SystemPrompt, "operators") || !strings.Contains(rendered.SystemPrompt, "maple-vector-citadel") {
		t.Fatalf("system prompt did not include variables: %q", rendered.SystemPrompt)
	}
	if rendered.UserPrompt != "Question: Where is the citadel?" {
		t.Fatalf("unexpected user prompt: %q", rendered.UserPrompt)
	}
}
