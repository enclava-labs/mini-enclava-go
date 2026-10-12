package httpx

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"enclava-go/internal/auth"
	"enclava-go/internal/prompts"
	"enclava-go/internal/store/sqlite"
)

func (a *App) handlePromptTemplates(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if a.promptSvc == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "prompt templates are not configured", "server_error", "not_configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		templates, err := a.promptSvc.List(r.Context())
		if err != nil {
			log.Printf("failed to list prompt templates: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "prompt_templates_list_failed")
			return
		}
		templates, err = a.visiblePromptTemplates(r.Context(), principal, templates)
		if err != nil {
			log.Printf("failed to filter prompt templates: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "prompt_templates_list_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"prompt_templates": templates})
	case http.MethodPost:
		if !auth.HasScope(principal, "chatbot.manage") {
			WriteForbidden(w, "missing required scope: chatbot.manage")
			return
		}
		var input prompts.CreateTemplateInput
		if err := DecodeJSON(r, &input); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		t, err := a.promptSvc.Create(r.Context(), input)
		if err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "prompt_template_invalid")
			return
		}
		WriteJSON(w, http.StatusCreated, t)
	default:
		WriteMethodNotAllowed(w)
	}
}

func (a *App) handlePromptTemplateByID(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if a.promptSvc == nil {
		WriteOpenAIError(w, http.StatusNotImplemented, "prompt templates are not configured", "server_error", "not_configured")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/prompt-templates/"), "/")
	if id == "" {
		WriteOpenAIError(w, http.StatusBadRequest, "template id is required", "invalid_request_error", "missing_template")
		return
	}
	switch r.Method {
	case http.MethodGet:
		t, err := a.promptSvc.Get(r.Context(), id)
		if err != nil {
			writePromptNotFoundOrError(w, err, "prompt_template_fetch_failed")
			return
		}
		visible, err := a.promptTemplateVisible(r.Context(), principal, t.ID)
		if err != nil {
			log.Printf("prompt template error: %v", err)
			WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "prompt_template_fetch_failed")
			return
		}
		if !visible {
			WriteOpenAIError(w, http.StatusNotFound, "prompt template not found", "invalid_request_error", "not_found")
			return
		}
		WriteJSON(w, http.StatusOK, t)
	case http.MethodPut:
		if !auth.HasScope(principal, "chatbot.manage") {
			WriteForbidden(w, "missing required scope: chatbot.manage")
			return
		}
		var input prompts.UpdateTemplateInput
		if err := DecodeJSON(r, &input); err != nil {
			WriteOpenAIError(w, http.StatusBadRequest, "invalid JSON payload", "invalid_request_error", "invalid_json")
			return
		}
		t, err := a.promptSvc.Update(r.Context(), id, input)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				WriteOpenAIError(w, http.StatusNotFound, "prompt template not found", "invalid_request_error", "not_found")
				return
			}
			WriteOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "prompt_template_invalid")
			return
		}
		WriteJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		if !auth.HasScope(principal, "chatbot.manage") {
			WriteForbidden(w, "missing required scope: chatbot.manage")
			return
		}
		if err := a.promptSvc.Delete(r.Context(), id); err != nil {
			writePromptNotFoundOrError(w, err, "prompt_template_delete_failed")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		WriteMethodNotAllowed(w)
	}
}

func (a *App) visiblePromptTemplates(ctx context.Context, principal auth.Principal, templates []sqlite.PromptTemplate) ([]sqlite.PromptTemplate, error) {
	if chatbotAllowlistOpen(principal) {
		return templates, nil
	}
	allowed, err := a.allowedPromptTemplateIDs(ctx, principal)
	if err != nil {
		return nil, err
	}
	out := make([]sqlite.PromptTemplate, 0, len(templates))
	for _, template := range templates {
		if _, ok := allowed[template.ID]; ok {
			out = append(out, template)
		}
	}
	return out, nil
}

func (a *App) promptTemplateVisible(ctx context.Context, principal auth.Principal, templateID string) (bool, error) {
	if chatbotAllowlistOpen(principal) {
		return true, nil
	}
	allowed, err := a.allowedPromptTemplateIDs(ctx, principal)
	if err != nil {
		return false, err
	}
	_, ok := allowed[templateID]
	return ok, nil
}

func (a *App) allowedPromptTemplateIDs(ctx context.Context, principal auth.Principal) (map[string]struct{}, error) {
	allowed := map[string]struct{}{}
	if a.chatbotSvc == nil {
		return allowed, nil
	}
	bots, err := a.chatbotSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, bot := range bots {
		if !chatbotVisible(principal, bot) {
			continue
		}
		id := strings.TrimSpace(bot.PromptTemplateID)
		if id == "" {
			continue
		}
		allowed[id] = struct{}{}
	}
	return allowed, nil
}

func writePromptNotFoundOrError(w http.ResponseWriter, err error, code string) {
	if errors.Is(err, sql.ErrNoRows) {
		WriteOpenAIError(w, http.StatusNotFound, "prompt template not found", "invalid_request_error", "not_found")
		return
	}
	log.Printf("prompt template error: %v", err)
	WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", code)
}
