package extract

import (
	"context"
	"database/sql"
	"encoding/json"
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
	request  []byte
}

func (f *fakeProvider) ListModels(ctx context.Context) (llm.ModelsResponse, error) {
	return llm.ModelsResponse{}, nil
}
func (f *fakeProvider) GetModel(ctx context.Context, modelID string) (llm.Model, error) {
	return llm.Model{}, nil
}
func (f *fakeProvider) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	f.request = append([]byte(nil), body...)
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

func TestProcessAppliesOutputSchemaResponseFormatAndValidation(t *testing.T) {
	template := sqlite.ExtractTemplate{
		ID:            "invoice schema",
		Name:          "template",
		SystemPrompt:  "sys",
		UserPrompt:    "user",
		Model:         "allowed-model",
		IsActive:      true,
		ContextSchema: map[string]any{},
		OutputSchema: map[string]any{
			"type":     "object",
			"required": []any{"total"},
			"properties": map[string]any{
				"total": map[string]any{"type": "number"},
			},
		},
	}
	provider := &fakeProvider{
		response: []byte(`{"id":"c","model":"allowed-model","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3},"choices":[{"message":{"content":"{\"total\":\"12.99\"}"}}]}`),
	}
	svc := NewService(
		&fakeTemplatesRepo{template: template},
		&fakeSettingsRepo{},
		&fakeJobsRepo{},
		provider,
		5,
	)

	result, err := svc.Process(context.Background(), 1, ProcessInput{
		TemplateID: "invoice schema",
		FileName:   "doc.pdf",
		MimeType:   "application/pdf",
		Bytes:      []byte("fake"),
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected schema validation failure")
	}
	if len(result.ValidationErrors) == 0 {
		t.Fatalf("expected validation errors")
	}

	var request map[string]any
	if err := json.Unmarshal(provider.request, &request); err != nil {
		t.Fatalf("decode provider request: %v", err)
	}
	responseFormat, _ := request["response_format"].(map[string]any)
	if responseFormat["type"] != "json_schema" {
		t.Fatalf("expected json_schema response_format, got %#v", responseFormat)
	}
}

func TestValidateAgainstSchemaNullableObjectAndArrayAcceptNull(t *testing.T) {
	objectSchema := map[string]any{
		"type": []any{"object", "null"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}
	if errs := validateAgainstSchema(nil, objectSchema, "$"); len(errs) != 0 {
		t.Fatalf("nullable object schema rejected null: %v", errs)
	}

	arraySchema := map[string]any{
		"type":  []any{"array", "null"},
		"items": map[string]any{"type": "string"},
	}
	if errs := validateAgainstSchema(nil, arraySchema, "$"); len(errs) != 0 {
		t.Fatalf("nullable array schema rejected null: %v", errs)
	}

	enumSchema := map[string]any{
		"type": []any{"object", "null"},
		"enum": []any{"keep"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	errs := validateAgainstSchema(nil, enumSchema, "$")
	if len(errs) != 1 || errs[0] != "$ must be one of the declared enum values" {
		t.Fatalf("expected enum failure only, got %v", errs)
	}
}

func TestValidateAgainstSchemaNonNullObjectAndArray(t *testing.T) {
	objectSchema := map[string]any{
		"type":     []any{"object", "null"},
		"required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	}
	if errs := validateAgainstSchema(map[string]any{"name": "ada"}, objectSchema, "$"); len(errs) != 0 {
		t.Fatalf("valid object rejected: %v", errs)
	}
	if errs := validateAgainstSchema(map[string]any{"name": 1, "extra": true}, objectSchema, "$"); len(errs) == 0 {
		t.Fatal("expected non-null object validation errors")
	}

	arraySchema := map[string]any{
		"type":  []any{"array", "null"},
		"items": map[string]any{"type": "string"},
	}
	if errs := validateAgainstSchema([]any{"a", "b"}, arraySchema, "$"); len(errs) != 0 {
		t.Fatalf("valid array rejected: %v", errs)
	}
	if errs := validateAgainstSchema([]any{"a", 1}, arraySchema, "$"); len(errs) == 0 {
		t.Fatal("expected non-null array item validation error")
	}
}

func TestValidateAgainstSchemaNullRejectedWhenNotNullable(t *testing.T) {
	errs := validateAgainstSchema(nil, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}, "$")
	if len(errs) != 1 || errs[0] != "$ must be object" {
		t.Fatalf("expected non-nullable object type failure, got %v", errs)
	}

	errs = validateAgainstSchema(nil, map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}, "$")
	if len(errs) != 1 || errs[0] != "$ must be array" {
		t.Fatalf("expected non-nullable array type failure, got %v", errs)
	}
}

func TestValidateAgainstSchemaRequiredAndNullableProperty(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"title"},
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
			"meta": map[string]any{
				"type": []any{"object", "null"},
				"properties": map[string]any{
					"k": map[string]any{"type": "string"},
				},
			},
			"tags": map[string]any{
				"type":  []any{"array", "null"},
				"items": map[string]any{"type": "string"},
			},
		},
	}
	errs := validateAgainstSchema(map[string]any{"meta": nil, "tags": nil}, schema, "$")
	if len(errs) != 1 || errs[0] != "$.title is required" {
		t.Fatalf("expected missing required key, got %v", errs)
	}
	if errs := validateAgainstSchema(map[string]any{
		"title": "invoice",
		"meta":  nil,
		"tags":  nil,
	}, schema, "$"); len(errs) != 0 {
		t.Fatalf("present nullable properties rejected null: %v", errs)
	}
}
