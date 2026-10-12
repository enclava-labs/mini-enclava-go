package prompts

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"text/template"

	"enclava-go/internal/store/sqlite"
)

type Repository interface {
	Create(ctx context.Context, t sqlite.PromptTemplate) error
	List(ctx context.Context) ([]sqlite.PromptTemplate, error)
	GetByID(ctx context.Context, id string) (sqlite.PromptTemplate, error)
	Update(ctx context.Context, t sqlite.PromptTemplate) error
	Delete(ctx context.Context, id string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateTemplateInput) (sqlite.PromptTemplate, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return sqlite.PromptTemplate{}, errors.New("name is required")
	}
	userPrompt := strings.TrimSpace(input.UserPrompt)
	if userPrompt == "" {
		userPrompt = "{{.user_input}}"
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = newID("prompt")
	}
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	t := sqlite.PromptTemplate{
		ID:              id,
		Name:            name,
		Description:     strings.TrimSpace(input.Description),
		SystemPrompt:    strings.TrimSpace(input.SystemPrompt),
		UserPrompt:      userPrompt,
		VariablesSchema: nonNilMap(input.VariablesSchema),
		IsDefault:       input.IsDefault,
		IsActive:        active,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return sqlite.PromptTemplate{}, err
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]sqlite.PromptTemplate, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (sqlite.PromptTemplate, error) {
	return s.repo.GetByID(ctx, strings.TrimSpace(id))
}

func (s *Service) Update(ctx context.Context, id string, input UpdateTemplateInput) (sqlite.PromptTemplate, error) {
	t, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return sqlite.PromptTemplate{}, err
	}
	if input.Name != nil {
		t.Name = strings.TrimSpace(*input.Name)
	}
	if t.Name == "" {
		return sqlite.PromptTemplate{}, errors.New("name is required")
	}
	if input.Description != nil {
		t.Description = strings.TrimSpace(*input.Description)
	}
	if input.SystemPrompt != nil {
		t.SystemPrompt = strings.TrimSpace(*input.SystemPrompt)
	}
	if input.UserPrompt != nil {
		t.UserPrompt = strings.TrimSpace(*input.UserPrompt)
	}
	if t.UserPrompt == "" {
		t.UserPrompt = "{{.user_input}}"
	}
	if input.VariablesSchema != nil {
		t.VariablesSchema = input.VariablesSchema
	}
	if input.IsDefault != nil {
		t.IsDefault = *input.IsDefault
	}
	if input.IsActive != nil {
		t.IsActive = *input.IsActive
	}
	if err := s.repo.Update(ctx, t); err != nil {
		return sqlite.PromptTemplate{}, err
	}
	return s.repo.GetByID(ctx, t.ID)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, strings.TrimSpace(id))
}

func Render(t sqlite.PromptTemplate, input RenderInput) (RenderedPrompt, error) {
	vars := nonNilMap(input.Variables)
	vars["user_input"] = input.UserInput
	vars["question"] = input.UserInput
	vars["rag_context"] = input.RAGContext

	system, err := renderTemplate(t.ID+":system", t.SystemPrompt, vars)
	if err != nil {
		return RenderedPrompt{}, err
	}
	user, err := renderTemplate(t.ID+":user", t.UserPrompt, vars)
	if err != nil {
		return RenderedPrompt{}, err
	}
	return RenderedPrompt{SystemPrompt: strings.TrimSpace(system), UserPrompt: strings.TrimSpace(user)}, nil
}

func renderTemplate(name, body string, vars map[string]any) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=zero").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func nonNilMap(v map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range v {
		out[key] = value
	}
	return out
}
