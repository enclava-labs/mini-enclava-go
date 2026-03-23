package extract

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"enclava-go/internal/llm"
	"enclava-go/internal/store/sqlite"
)

type fakeTemplatesRepo struct {
	template sqlite.ExtractTemplate
}

func (f *fakeTemplatesRepo) List(ctx context.Context) ([]sqlite.ExtractTemplate, error) {
	return []sqlite.ExtractTemplate{f.template}, nil
}
func (f *fakeTemplatesRepo) GetByID(ctx context.Context, id string) (sqlite.ExtractTemplate, error) {
	if f.template.ID == "" || f.template.ID != id {
		return sqlite.ExtractTemplate{}, sql.ErrNoRows
	}
	return f.template, nil
}
func (f *fakeTemplatesRepo) Upsert(ctx context.Context, t sqlite.ExtractTemplate) error { return nil }
func (f *fakeTemplatesRepo) Delete(ctx context.Context, id string) error                { return nil }
func (f *fakeTemplatesRepo) ReplaceDefaults(ctx context.Context, templates []sqlite.ExtractTemplate) error {
	return nil
}

type fakeSettingsRepo struct {
	settings sqlite.ExtractSettings
}

func (f *fakeSettingsRepo) Get(ctx context.Context) (sqlite.ExtractSettings, error) {
	return f.settings, nil
}
func (f *fakeSettingsRepo) Update(ctx context.Context, defaultModel string, maxFileSizeMB int64) (sqlite.ExtractSettings, error) {
	f.settings.DefaultModel = defaultModel
	return f.settings, nil
}

type fakeJobsRepo struct {
	createCalls int
}

func (f *fakeJobsRepo) Create(ctx context.Context, job sqlite.ExtractJob) error {
	f.createCalls++
	return nil
}
func (f *fakeJobsRepo) UpdateStatus(ctx context.Context, jobID, status, modelUsed, errorMessage string, markCompleted bool) error {
	return nil
}
func (f *fakeJobsRepo) List(ctx context.Context, apiKeyID int64, limit, offset int) ([]sqlite.ExtractJob, int, error) {
	return nil, 0, nil
}
func (f *fakeJobsRepo) Get(ctx context.Context, apiKeyID int64, jobID string) (sqlite.ExtractJob, error) {
	return sqlite.ExtractJob{}, nil
}
func (f *fakeJobsRepo) SaveResult(ctx context.Context, result sqlite.ExtractResult) error { return nil }
func (f *fakeJobsRepo) GetResultByJobID(ctx context.Context, jobID string) (sqlite.ExtractResult, error) {
	return sqlite.ExtractResult{}, nil
}

type fakeProvider struct {
	response []byte
}

func (f *fakeProvider) ListModels(ctx context.Context) (llm.ModelsResponse, error) {
	return llm.ModelsResponse{}, nil
}
func (f *fakeProvider) GetModel(ctx context.Context, modelID string) (llm.Model, error) {
	return llm.Model{}, nil
}
func (f *fakeProvider) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	return f.response, nil
}
func (f *fakeProvider) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeProvider) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	return nil, nil
}

func TestParseJSONBody_CodeFence(t *testing.T) {
	payload := "```json\n{\"a\":1}\n```"
	parsed, err := parseJSONBody(payload)
	if err != nil {
		t.Fatalf("expected parse success, got err=%v", err)
	}
	if parsed["a"].(float64) != 1 {
		t.Fatalf("expected a=1, got %v", parsed["a"])
	}
}

func TestValidateContextValues_NestedRejected(t *testing.T) {
	err := validateContextValues(map[string]any{"nested": map[string]any{"x": 1}})
	if err == nil {
		t.Fatalf("expected error for nested context value")
	}
}

func TestProcessRejectsDisallowedModel(t *testing.T) {
	template := sqlite.ExtractTemplate{
		ID:            "tpl",
		Name:          "template",
		SystemPrompt:  "sys",
		UserPrompt:    "user",
		Model:         "blocked-model",
		IsActive:      true,
		ContextSchema: map[string]any{},
	}
	svc := NewService(
		&fakeTemplatesRepo{template: template},
		&fakeSettingsRepo{},
		&fakeJobsRepo{},
		&fakeProvider{},
		5,
	)

	_, err := svc.Process(context.Background(), 1, ProcessInput{
		TemplateID: "tpl",
		FileName:   "doc.pdf",
		MimeType:   "application/pdf",
		Bytes:      []byte("fake"),
	}, map[string]struct{}{
		"allowed-model": {},
	})
	if err == nil {
		t.Fatalf("expected error for disallowed model")
	}
}

func TestProcessAllowsAllowedModel(t *testing.T) {
	template := sqlite.ExtractTemplate{
		ID:            "tpl",
		Name:          "template",
		SystemPrompt:  "sys",
		UserPrompt:    "user",
		Model:         "allowed-model",
		IsActive:      true,
		ContextSchema: map[string]any{},
	}
	provider := &fakeProvider{
		response: []byte(`{"id":"c","model":"allowed-model","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3},"choices":[{"message":{"content":"{\"a\":1}"}}]}`),
	}
	svc := NewService(
		&fakeTemplatesRepo{template: template},
		&fakeSettingsRepo{},
		&fakeJobsRepo{},
		provider,
		5,
	)

	result, err := svc.Process(context.Background(), 1, ProcessInput{
		TemplateID: "tpl",
		FileName:   "doc.pdf",
		MimeType:   "application/pdf",
		Bytes:      []byte("fake"),
	}, map[string]struct{}{
		"allowed-model": {},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ModelUsed != "allowed-model" {
		t.Fatalf("expected model used allowed-model, got %q", result.ModelUsed)
	}
	if result.TokensUsed != 3 {
		t.Fatalf("expected token usage 3, got %d", result.TokensUsed)
	}
}
