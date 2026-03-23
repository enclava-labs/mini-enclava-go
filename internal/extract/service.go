package extract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"enclava-go/internal/llm"
	"enclava-go/internal/store/sqlite"
)

type TemplateRepository interface {
	List(ctx context.Context) ([]sqlite.ExtractTemplate, error)
	GetByID(ctx context.Context, id string) (sqlite.ExtractTemplate, error)
	Upsert(ctx context.Context, t sqlite.ExtractTemplate) error
	Delete(ctx context.Context, id string) error
	ReplaceDefaults(ctx context.Context, templates []sqlite.ExtractTemplate) error
}

type SettingsRepository interface {
	Get(ctx context.Context) (sqlite.ExtractSettings, error)
	Update(ctx context.Context, defaultModel string, maxFileSizeMB int64) (sqlite.ExtractSettings, error)
}

type JobsRepository interface {
	Create(ctx context.Context, job sqlite.ExtractJob) error
	UpdateStatus(ctx context.Context, jobID, status, modelUsed, errorMessage string, markCompleted bool) error
	List(ctx context.Context, apiKeyID int64, limit, offset int) ([]sqlite.ExtractJob, int, error)
	Get(ctx context.Context, apiKeyID int64, jobID string) (sqlite.ExtractJob, error)
	SaveResult(ctx context.Context, result sqlite.ExtractResult) error
	GetResultByJobID(ctx context.Context, jobID string) (sqlite.ExtractResult, error)
}

type Service struct {
	templatesRepo TemplateRepository
	settingsRepo  SettingsRepository
	jobsRepo      JobsRepository
	provider      llm.Provider
	maxUploadMB   int64
}

func NewService(
	templatesRepo TemplateRepository,
	settingsRepo SettingsRepository,
	jobsRepo JobsRepository,
	provider llm.Provider,
	maxUploadMB int64,
) *Service {
	return &Service{
		templatesRepo: templatesRepo,
		settingsRepo:  settingsRepo,
		jobsRepo:      jobsRepo,
		provider:      provider,
		maxUploadMB:   maxUploadMB,
	}
}

func (s *Service) SeedDefaults(ctx context.Context) error {
	return s.templatesRepo.ReplaceDefaults(ctx, DefaultTemplates())
}

func (s *Service) Process(ctx context.Context, apiKeyID int64, input ProcessInput, allowedModels map[string]struct{}) (ProcessOutput, error) {
	start := time.Now()
	if err := s.validateInput(input); err != nil {
		return ProcessOutput{}, err
	}

	template, err := s.templatesRepo.GetByID(ctx, input.TemplateID)
	if err != nil {
		return ProcessOutput{}, fmt.Errorf("load template: %w", err)
	}
	if !template.IsActive {
		return ProcessOutput{}, errors.New("template is inactive")
	}

	jobID := newID("job")
	job := sqlite.ExtractJob{
		ID:            jobID,
		APIKeyID:      apiKeyID,
		TemplateID:    template.ID,
		Status:        StatusPending,
		FileName:      input.FileName,
		FileMimeType:  input.MimeType,
		FileSizeBytes: int64(len(input.Bytes)),
		NumPages:      1,
	}
	if err := s.jobsRepo.Create(ctx, job); err != nil {
		return ProcessOutput{}, fmt.Errorf("create job: %w", err)
	}
	_ = s.jobsRepo.UpdateStatus(ctx, jobID, StatusProcessing, "", "", false)

	model, err := s.resolveModel(ctx, template)
	if err != nil {
		_ = s.jobsRepo.UpdateStatus(ctx, jobID, StatusFailed, "", err.Error(), true)
		return ProcessOutput{}, err
	}
	if !isModelAllowed(model, allowedModels) {
		err := fmt.Errorf("model is not allowed for this API key")
		_ = s.jobsRepo.UpdateStatus(ctx, jobID, StatusFailed, model, err.Error(), true)
		return ProcessOutput{}, err
	}
	promptSystem, promptUser := interpolate(template.SystemPrompt, template.UserPrompt, input.Context)
	requestPayload := s.buildProviderRequest(model, promptSystem, promptUser, input)
	requestBytes, err := json.Marshal(requestPayload)
	if err != nil {
		_ = s.jobsRepo.UpdateStatus(ctx, jobID, StatusFailed, model, err.Error(), true)
		return ProcessOutput{}, fmt.Errorf("marshal provider request: %w", err)
	}

	chatRespRaw, err := s.provider.CreateChatCompletion(ctx, requestBytes)
	if err != nil {
		_ = s.jobsRepo.UpdateStatus(ctx, jobID, StatusFailed, model, err.Error(), true)
		return ProcessOutput{}, fmt.Errorf("provider chat completion: %w", err)
	}

	parsed, rawContent, usage, parseErr := parseProviderExtraction(chatRespRaw)
	if parseErr != nil {
		if parsed == nil {
			parsed = map[string]any{}
		}
		parsed["_parse_error"] = parseErr.Error()
	}
	validationErrors, validationWarnings := s.validateOutput(parsed, template)

	status := StatusCompleted
	if len(validationErrors) > 0 {
		status = StatusCompletedWithErrors
	}

	result := sqlite.ExtractResult{
		ID:                 newID("res"),
		JobID:              jobID,
		Data:               parsed,
		RawResponse:        rawContent,
		ValidationErrors:   validationErrors,
		ValidationWarnings: validationWarnings,
		TokensUsed:         usage.Total,
		CostCents:          0,
	}
	if err := s.jobsRepo.SaveResult(ctx, result); err != nil {
		_ = s.jobsRepo.UpdateStatus(ctx, jobID, StatusFailed, model, err.Error(), true)
		return ProcessOutput{}, fmt.Errorf("save extract result: %w", err)
	}
	_ = s.jobsRepo.UpdateStatus(ctx, jobID, status, model, "", true)

	return ProcessOutput{
		Success:              len(validationErrors) == 0,
		JobID:                jobID,
		ModelUsed:            model,
		Data:                 parsed,
		RawResponse:          rawContent,
		ValidationErrors:     validationErrors,
		ValidationWarnings:   validationWarnings,
		ProcessingTimeMS:     time.Since(start).Milliseconds(),
		PromptTokensUsed:     usage.Prompt,
		CompletionTokensUsed: usage.Completion,
		TokensUsed:           usage.Total,
		CostCents:            0,
	}, nil
}

func isModelAllowed(model string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[model]
	return ok
}

func (s *Service) ListJobs(ctx context.Context, apiKeyID int64, limit, offset int) ([]sqlite.ExtractJob, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.jobsRepo.List(ctx, apiKeyID, limit, offset)
}

func (s *Service) GetJob(ctx context.Context, apiKeyID int64, jobID string) (JobDetail, error) {
	job, err := s.jobsRepo.Get(ctx, apiKeyID, jobID)
	if err != nil {
		return JobDetail{}, err
	}
	result, err := s.jobsRepo.GetResultByJobID(ctx, jobID)
	if err != nil {
		return JobDetail{Job: job}, nil
	}
	return JobDetail{Job: job, Result: &result}, nil
}

func (s *Service) ListTemplates(ctx context.Context) ([]sqlite.ExtractTemplate, error) {
	return s.templatesRepo.List(ctx)
}

func (s *Service) GetTemplate(ctx context.Context, id string) (sqlite.ExtractTemplate, error) {
	return s.templatesRepo.GetByID(ctx, id)
}

func (s *Service) UpsertTemplate(ctx context.Context, t sqlite.ExtractTemplate) error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("template id is required")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("template name is required")
	}
	if strings.TrimSpace(t.SystemPrompt) == "" || strings.TrimSpace(t.UserPrompt) == "" {
		return errors.New("system_prompt and user_prompt are required")
	}
	if t.ContextSchema == nil {
		t.ContextSchema = map[string]any{}
	}
	if !isPrimitiveSchema(t.ContextSchema) {
		return errors.New("context_schema must be a flat object with primitive type definitions")
	}
	return s.templatesRepo.Upsert(ctx, t)
}

func (s *Service) DeleteTemplate(ctx context.Context, id string) error {
	return s.templatesRepo.Delete(ctx, id)
}

func (s *Service) ResetDefaultTemplates(ctx context.Context) error {
	return s.templatesRepo.ReplaceDefaults(ctx, DefaultTemplates())
}

func (s *Service) GetSettings(ctx context.Context) (sqlite.ExtractSettings, error) {
	return s.settingsRepo.Get(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, defaultModel string, maxFileSizeMB int64) (sqlite.ExtractSettings, error) {
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = s.maxUploadMB
	}
	return s.settingsRepo.Update(ctx, defaultModel, maxFileSizeMB)
}

func (s *Service) ModelsForExtract(ctx context.Context) ([]llm.Model, error) {
	modelsResp, err := s.provider.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]llm.Model, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		if llm.SupportsVisionOrDocument(m) {
			models = append(models, m)
		}
	}
	// Some providers don't populate capability metadata; fall back to all models.
	if len(models) == 0 {
		return modelsResp.Data, nil
	}
	return models, nil
}

func (s *Service) validateInput(input ProcessInput) error {
	if strings.TrimSpace(input.TemplateID) == "" {
		return errors.New("template_id is required")
	}
	if strings.TrimSpace(input.FileName) == "" {
		return errors.New("file is required")
	}
	if len(input.Bytes) == 0 {
		return errors.New("uploaded file is empty")
	}
	if int64(len(input.Bytes)) > s.maxUploadMB*1024*1024 {
		return fmt.Errorf("file exceeds max size of %dMB", s.maxUploadMB)
	}
	ext := strings.ToLower(filepath.Ext(input.FileName))
	allowed := map[string]struct{}{".pdf": {}, ".jpg": {}, ".jpeg": {}, ".png": {}}
	if _, ok := allowed[ext]; !ok {
		return fmt.Errorf("unsupported file type: %s", ext)
	}
	if err := validateContextValues(input.Context); err != nil {
		return err
	}
	return nil
}

func (s *Service) resolveModel(ctx context.Context, t sqlite.ExtractTemplate) (string, error) {
	if strings.TrimSpace(t.Model) != "" {
		return t.Model, nil
	}
	settings, err := s.settingsRepo.Get(ctx)
	if err == nil && strings.TrimSpace(settings.DefaultModel) != "" {
		return settings.DefaultModel, nil
	}

	modelsResp, err := s.provider.ListModels(ctx)
	if err != nil {
		return "", fmt.Errorf("list models for fallback: %w", err)
	}
	for _, m := range modelsResp.Data {
		if llm.SupportsVisionOrDocument(m) {
			return m.ID, nil
		}
	}
	if len(modelsResp.Data) > 0 {
		return modelsResp.Data[0].ID, nil
	}
	return "", errors.New("no models available")
}

func (s *Service) buildProviderRequest(model, systemPrompt, userPrompt string, input ProcessInput) map[string]any {
	b64 := base64.StdEncoding.EncodeToString(input.Bytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", input.MimeType, b64)

	content := []map[string]any{}
	mime := strings.ToLower(input.MimeType)
	if strings.Contains(mime, "image/") {
		content = append(content, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url":    dataURL,
				"detail": "high",
			},
		})
	} else {
		content = append(content, map[string]any{
			"type": "input_file",
			"input_file": map[string]any{
				"filename":  input.FileName,
				"file_data": dataURL,
			},
		})
	}
	content = append(content, map[string]any{
		"type": "text",
		"text": userPrompt,
	})

	return map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": content},
		},
		"response_format": map[string]any{"type": "json_object"},
		"temperature":     0,
	}
}

func interpolate(systemPrompt, userPrompt string, context map[string]any) (string, string) {
	if context == nil {
		return systemPrompt, userPrompt
	}
	for k, v := range context {
		placeholder := "{" + k + "}"
		strVal := fmt.Sprint(v)
		systemPrompt = strings.ReplaceAll(systemPrompt, placeholder, strVal)
		userPrompt = strings.ReplaceAll(userPrompt, placeholder, strVal)
	}
	return systemPrompt, userPrompt
}

func validateContextValues(ctx map[string]any) error {
	for key, value := range ctx {
		if strings.TrimSpace(key) == "" {
			return errors.New("context keys must be non-empty strings")
		}
		switch value.(type) {
		case nil, string, float64, bool, int, int64, uint64:
			continue
		default:
			return fmt.Errorf("context key %q has non-primitive value", key)
		}
	}
	return nil
}

func isPrimitiveSchema(schema map[string]any) bool {
	for _, v := range schema {
		if _, ok := v.(map[string]any); !ok {
			return false
		}
	}
	return true
}

type usageInfo struct {
	Prompt     int
	Completion int
	Total      int
}

func parseProviderExtraction(raw []byte) (map[string]any, string, usageInfo, error) {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, "", usageInfo{}, fmt.Errorf("decode provider response: %w", err)
	}

	usage := usageInfo{}
	if usageRaw, ok := envelope["usage"].(map[string]any); ok {
		usage.Prompt = int(toFloat(usageRaw["prompt_tokens"]))
		usage.Completion = int(toFloat(usageRaw["completion_tokens"]))
		usage.Total = int(toFloat(usageRaw["total_tokens"]))
	}

	choicesRaw, ok := envelope["choices"].([]any)
	if !ok || len(choicesRaw) == 0 {
		return map[string]any{}, "", usage, errors.New("provider returned no choices")
	}
	choice, _ := choicesRaw[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	contentValue := message["content"]
	rawContent := extractContentText(contentValue)
	parsed, err := parseJSONBody(rawContent)
	if err != nil {
		return map[string]any{}, rawContent, usage, err
	}
	return parsed, rawContent, usage, nil
}

func extractContentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := m["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func parseJSONBody(content string) (map[string]any, error) {
	t := strings.TrimSpace(content)
	if strings.HasPrefix(t, "```json") {
		t = strings.TrimPrefix(t, "```json")
	}
	if strings.HasPrefix(t, "```") {
		t = strings.TrimPrefix(t, "```")
	}
	if strings.HasSuffix(t, "```") {
		t = strings.TrimSuffix(t, "```")
	}
	t = strings.TrimSpace(t)
	if t == "" {
		return map[string]any{}, errors.New("empty model response")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(t), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) validateOutput(data map[string]any, template sqlite.ExtractTemplate) ([]string, []string) {
	errorsList := []string{}
	warnings := []string{}
	if data == nil || len(data) == 0 {
		errorsList = append(errorsList, "empty extraction output")
		return errorsList, warnings
	}
	if _, hasParseErr := data["_parse_error"]; hasParseErr {
		errorsList = append(errorsList, "model output is not valid JSON")
	}
	if template.ID == "detailed_invoice" {
		if _, ok := data["invoice_number"]; !ok {
			warnings = append(warnings, "missing invoice_number")
		}
		if _, ok := data["total_amount"]; !ok {
			warnings = append(warnings, "missing total_amount")
		}
	}
	return errorsList, warnings
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
