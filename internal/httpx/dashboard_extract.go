package httpx

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"enclava-go/internal/extract"
	"enclava-go/internal/store/sqlite"
)

// handleDashboardExtractTemplates handles GET (list) and POST (create) for templates.
func (a *App) handleDashboardExtractTemplates(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	switch r.Method {
	case http.MethodGet:
		if a.extractTemplates == nil {
			WriteJSON(w, http.StatusOK, map[string]any{"templates": []any{}})
			return
		}
		templates, err := a.extractTemplates.List(r.Context())
		if err != nil {
			log.Printf("dashboard: list extract templates: %v", err)
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"templates": templates})
	case http.MethodPost:
		if a.extractSvc == nil {
			WriteJSON(w, http.StatusNotImplemented, map[string]any{"error": "extract not configured"})
			return
		}
		var payload sqlite.ExtractTemplate
		if err := DecodeJSON(r, &payload); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
			return
		}
		if err := a.extractSvc.UpsertTemplate(r.Context(), payload); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		tpl, err := a.extractSvc.GetTemplate(r.Context(), payload.ID)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusCreated, tpl)
	default:
		WriteMethodNotAllowed(w)
	}
}

// handleDashboardExtractTemplateByID handles PUT (update) and DELETE for a specific template.
func (a *App) handleDashboardExtractTemplateByID(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if a.extractSvc == nil {
		WriteJSON(w, http.StatusNotImplemented, map[string]any{"error": "extract not configured"})
		return
	}
	templateID := strings.TrimPrefix(r.URL.Path, "/dashboard/partials/extract/templates/")
	if strings.TrimSpace(templateID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "template id is required"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		existing, err := a.extractSvc.GetTemplate(r.Context(), templateID)
		if err != nil {
			WriteJSON(w, http.StatusNotFound, map[string]any{"error": "template not found"})
			return
		}
		var patch map[string]any
		body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		defer func() { _ = r.Body.Close() }()
		if err := json.Unmarshal(body, &patch); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
			return
		}
		merged := dashboardMergeTemplate(existing, patch)
		if err := a.extractSvc.UpsertTemplate(r.Context(), merged); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		tpl, err := a.extractSvc.GetTemplate(r.Context(), templateID)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, tpl)
	case http.MethodDelete:
		if err := a.extractSvc.DeleteTemplate(r.Context(), templateID); err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": templateID})
	default:
		WriteMethodNotAllowed(w)
	}
}

// handleDashboardExtractResetDefaults resets default templates.
func (a *App) handleDashboardExtractResetDefaults(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if r.Method != http.MethodPost {
		WriteMethodNotAllowed(w)
		return
	}
	if a.extractSvc == nil {
		WriteJSON(w, http.StatusNotImplemented, map[string]any{"error": "extract not configured"})
		return
	}
	if err := a.extractSvc.ResetDefaultTemplates(r.Context()); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDashboardExtractSettings handles GET (fetch) and PUT (update) for extract settings.
func (a *App) handleDashboardExtractSettings(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if a.extractSettings == nil {
		WriteJSON(w, http.StatusNotImplemented, map[string]any{"error": "extract settings not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := a.extractSettings.Get(r.Context())
		if err != nil {
			log.Printf("dashboard: get extract settings: %v", err)
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var payload struct {
			DefaultModel  string `json:"default_model"`
			MaxFileSizeMB int64  `json:"max_file_size_mb"`
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		defer func() { _ = r.Body.Close() }()
		if err := json.Unmarshal(body, &payload); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
			return
		}
		if payload.MaxFileSizeMB <= 0 {
			payload.MaxFileSizeMB = a.maxUploadMB
		}
		updated, err := a.extractSettings.Update(r.Context(), payload.DefaultModel, payload.MaxFileSizeMB)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, updated)
	default:
		WriteMethodNotAllowed(w)
	}
}

// handleDashboardExtractProcess handles multipart file upload and extraction.
func (a *App) handleDashboardExtractProcess(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if r.Method != http.MethodPost {
		WriteMethodNotAllowed(w)
		return
	}
	if a.extractSvc == nil {
		WriteJSON(w, http.StatusNotImplemented, map[string]any{"error": "extract not configured"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, a.maxUploadMB*1024*1024)
	if err := r.ParseMultipartForm(a.maxUploadMB * 1024 * 1024); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form or file too large"})
		return
	}

	templateID := strings.TrimSpace(r.FormValue("template_id"))
	if templateID == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "template_id is required"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "file is required"})
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read uploaded file"})
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}

	contextValues := map[string]any{}
	contextRaw := strings.TrimSpace(r.FormValue("context"))
	if contextRaw != "" {
		if err := json.Unmarshal([]byte(contextRaw), &contextValues); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "context must be valid JSON object"})
			return
		}
	}

	// Dashboard process uses apiKeyID=0 (admin context) and no model restrictions.
	result, err := a.extractSvc.Process(r.Context(), 0, extract.ProcessInput{
		TemplateID: templateID,
		Context:    contextValues,
		FileName:   filepath.Base(header.Filename),
		MimeType:   mimeType,
		Bytes:      content,
	}, nil)
	if err != nil {
		WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// handleDashboardExtractJobs returns recent extract jobs as JSON.
func (a *App) handleDashboardExtractJobs(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	if a.extractJobs == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"jobs": []any{}, "total": 0})
		return
	}
	limit := 20
	offset := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}
	jobs, total, err := a.extractJobs.ListAll(r.Context(), limit, offset)
	if err != nil {
		log.Printf("dashboard: list extract jobs: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "total": total, "limit": limit, "offset": offset})
}

// handleDashboardExtractModels returns available vision models as JSON.
func (a *App) handleDashboardExtractModels(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if r.Method != http.MethodGet {
		WriteMethodNotAllowed(w)
		return
	}
	if a.extractSvc == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"models": []any{}})
		return
	}
	models, err := a.extractSvc.ModelsForExtract(r.Context())
	if err != nil {
		log.Printf("dashboard: list extract models: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type modelInfo struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
	}
	out := make([]modelInfo, 0, len(models))
	for _, m := range models {
		name := m.ID
		if strings.TrimSpace(m.Object) != "" && m.Object != "model" {
			name = m.Object
		}
		out = append(out, modelInfo{ID: m.ID, Name: name})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"models": out})
}

// dashboardMergeTemplate merges a JSON patch into an existing template.
func dashboardMergeTemplate(existing sqlite.ExtractTemplate, patch map[string]any) sqlite.ExtractTemplate {
	if v, ok := patch["name"].(string); ok {
		existing.Name = v
	}
	if v, ok := patch["description"].(string); ok {
		existing.Description = v
	}
	if v, ok := patch["system_prompt"].(string); ok {
		existing.SystemPrompt = v
	}
	if v, ok := patch["user_prompt"].(string); ok {
		existing.UserPrompt = v
	}
	if v, ok := patch["model"].(string); ok {
		existing.Model = v
	}
	if v, ok := patch["is_default"].(bool); ok {
		existing.IsDefault = v
	}
	if v, ok := patch["is_active"].(bool); ok {
		existing.IsActive = v
	}
	if v, ok := patch["context_schema"].(map[string]any); ok {
		existing.ContextSchema = v
	}
	return existing
}
