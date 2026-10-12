package connectors

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"enclava-go/internal/rag"
	"enclava-go/internal/store/sqlite"
)

type Repository interface {
	CreateSource(ctx context.Context, source sqlite.ConnectorSource) error
	ListSources(ctx context.Context, sourceType string, limit, offset int) ([]sqlite.ConnectorSource, error)
	GetSource(ctx context.Context, id string) (sqlite.ConnectorSource, error)
	UpdateSource(ctx context.Context, source sqlite.ConnectorSource) error
	DeleteSource(ctx context.Context, id string) error
	CreateSyncJob(ctx context.Context, job sqlite.ConnectorSyncJob) error
	UpdateSyncJob(ctx context.Context, job sqlite.ConnectorSyncJob) error
	ListSyncJobs(ctx context.Context, sourceID, status string, limit, offset int) ([]sqlite.ConnectorSyncJob, error)
	GetSyncJob(ctx context.Context, id string) (sqlite.ConnectorSyncJob, error)
}

type RAGUploader interface {
	UploadDocument(ctx context.Context, access rag.Access, collectionID string, input rag.UploadDocumentInput) (rag.UploadDocumentOutput, error)
}

type Service struct {
	repo       Repository
	rag        RAGUploader
	httpClient *http.Client
	scheduleMu sync.Mutex
	// allowPrivateBaseURL permits loopback and private config.base_url hosts.
	// NewService leaves it false; tests enable it for httptest servers.
	allowPrivateBaseURL bool
}

func NewService(repo Repository, ragSvc RAGUploader) *Service {
	return &Service{
		repo: repo,
		rag:  ragSvc,
		httpClient: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: refuseConnectorRedirect,
		},
	}
}

func (s *Service) setAllowPrivateBaseURL(allow bool) {
	s.allowPrivateBaseURL = allow
}

func (s *Service) StartScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if _, err := s.RunScheduledSyncs(ctx, Access{IsAdmin: true}, now.UTC()); err != nil {
					log.Printf("connector scheduled sync failed: %v", err)
				}
			}
		}
	}()
}

func (s *Service) RunScheduledSyncs(ctx context.Context, access Access, now time.Time) (int, error) {
	if !s.scheduleMu.TryLock() {
		return 0, nil
	}
	defer s.scheduleMu.Unlock()

	started := 0
	offset := 0
	var firstErr error
	for {
		sources, err := s.repo.ListSources(ctx, "", 100, offset)
		if err != nil {
			return started, err
		}
		if len(sources) == 0 {
			break
		}
		for _, source := range sources {
			if !scheduledSourceDue(source, now) {
				continue
			}
			if _, err := s.SyncSource(ctx, access, source.ID); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				log.Printf("connector scheduled source sync failed source_id=%s: %v", source.ID, err)
				continue
			}
			started++
		}
		if len(sources) < 100 {
			break
		}
		offset += len(sources)
	}
	return started, firstErr
}

func (s *Service) CreateSource(ctx context.Context, input CreateSourceInput) (sqlite.ConnectorSource, error) {
	sourceType, err := normalizeSourceType(input.Type)
	if err != nil {
		return sqlite.ConnectorSource{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return sqlite.ConnectorSource{}, errors.New("name is required")
	}
	enabled := true
	if input.IsEnabled != nil {
		enabled = *input.IsEnabled
	}
	source := sqlite.ConnectorSource{
		ID:                 newID("connsrc"),
		Type:               sourceType,
		Name:               name,
		TargetCollectionID: strings.TrimSpace(input.TargetCollectionID),
		Config:             nonNilMap(input.Config),
		CredentialRef:      strings.TrimSpace(input.CredentialRef),
		Checkpoint:         map[string]any{},
		IsEnabled:          enabled,
	}
	if err := s.validateConnectorSource(ctx, source); err != nil {
		return sqlite.ConnectorSource{}, err
	}
	if err := s.repo.CreateSource(ctx, source); err != nil {
		return sqlite.ConnectorSource{}, err
	}
	return s.repo.GetSource(ctx, source.ID)
}

func (s *Service) ListSources(ctx context.Context, sourceType string, limit, offset int) ([]sqlite.ConnectorSource, error) {
	return s.repo.ListSources(ctx, strings.TrimSpace(sourceType), limit, offset)
}

func (s *Service) GetSource(ctx context.Context, id string) (sqlite.ConnectorSource, error) {
	return s.repo.GetSource(ctx, strings.TrimSpace(id))
}

func (s *Service) UpdateSource(ctx context.Context, id string, input UpdateSourceInput) (sqlite.ConnectorSource, error) {
	source, err := s.repo.GetSource(ctx, strings.TrimSpace(id))
	if err != nil {
		return sqlite.ConnectorSource{}, err
	}
	if input.Type != nil {
		sourceType, err := normalizeSourceType(*input.Type)
		if err != nil {
			return sqlite.ConnectorSource{}, err
		}
		source.Type = sourceType
	}
	if input.Name != nil {
		source.Name = strings.TrimSpace(*input.Name)
	}
	if source.Name == "" {
		return sqlite.ConnectorSource{}, errors.New("name is required")
	}
	if input.TargetCollectionID != nil {
		source.TargetCollectionID = strings.TrimSpace(*input.TargetCollectionID)
	}
	if input.Config != nil {
		source.Config = nonNilMap(input.Config)
	}
	if input.CredentialRef != nil {
		source.CredentialRef = strings.TrimSpace(*input.CredentialRef)
	}
	if input.Checkpoint != nil {
		source.Checkpoint = nonNilMap(input.Checkpoint)
	}
	if input.IsEnabled != nil {
		source.IsEnabled = *input.IsEnabled
	}
	if err := s.validateConnectorSource(ctx, source); err != nil {
		return sqlite.ConnectorSource{}, err
	}
	if err := s.repo.UpdateSource(ctx, source); err != nil {
		return sqlite.ConnectorSource{}, err
	}
	return s.repo.GetSource(ctx, source.ID)
}

func (s *Service) DeleteSource(ctx context.Context, id string) error {
	return s.repo.DeleteSource(ctx, strings.TrimSpace(id))
}

func (s *Service) SyncSource(ctx context.Context, access Access, sourceID string) (sqlite.ConnectorSyncJob, error) {
	source, err := s.repo.GetSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return sqlite.ConnectorSyncJob{}, err
	}
	if !source.IsEnabled {
		return sqlite.ConnectorSyncJob{}, errors.New("connector source is disabled")
	}
	now := time.Now().UTC()
	job := sqlite.ConnectorSyncJob{
		ID:                newID("connsync"),
		ConnectorSourceID: source.ID,
		Status:            JobStatusRunning,
		StartedAt:         &now,
		Stats:             map[string]any{},
	}
	if err := s.repo.CreateSyncJob(ctx, job); err != nil {
		return sqlite.ConnectorSyncJob{}, err
	}

	stats, checkpoint, syncErr := s.syncDocuments(ctx, access, source)
	completedAt := time.Now().UTC()
	job.CompletedAt = &completedAt
	job.Stats = stats
	if syncErr != nil {
		job.Status = JobStatusFailed
		job.ErrorMessage = syncErr.Error()
		_ = s.repo.UpdateSyncJob(ctx, job)
		stored, err := s.repo.GetSyncJob(ctx, job.ID)
		if err != nil {
			return sqlite.ConnectorSyncJob{}, err
		}
		return stored, syncErr
	}

	job.Status = JobStatusSucceeded
	if err := s.repo.UpdateSyncJob(ctx, job); err != nil {
		return sqlite.ConnectorSyncJob{}, err
	}
	source.Checkpoint = nonNilMap(source.Checkpoint)
	for key, value := range checkpoint {
		source.Checkpoint[key] = value
	}
	source.Checkpoint["last_job_id"] = job.ID
	source.Checkpoint["last_sync_status"] = job.Status
	source.LastSyncAt = &completedAt
	if err := s.repo.UpdateSource(ctx, source); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sqlite.ConnectorSyncJob{}, err
	}
	return s.repo.GetSyncJob(ctx, job.ID)
}

func (s *Service) ListSyncJobs(ctx context.Context, sourceID, status string, limit, offset int) ([]sqlite.ConnectorSyncJob, error) {
	return s.repo.ListSyncJobs(ctx, strings.TrimSpace(sourceID), strings.TrimSpace(status), limit, offset)
}

func (s *Service) GetSyncJob(ctx context.Context, id string) (sqlite.ConnectorSyncJob, error) {
	return s.repo.GetSyncJob(ctx, strings.TrimSpace(id))
}

func (s *Service) syncDocuments(ctx context.Context, access Access, source sqlite.ConnectorSource) (map[string]any, map[string]any, error) {
	if s.rag == nil {
		return map[string]any{"uploaded_documents": 0}, nil, errors.New("rag service is not configured")
	}
	if strings.TrimSpace(source.TargetCollectionID) == "" {
		return map[string]any{"uploaded_documents": 0}, nil, errors.New("target_collection_id is required for connector sync")
	}
	if err := s.validateConnectorSource(ctx, source); err != nil {
		return map[string]any{"uploaded_documents": 0}, nil, err
	}
	documents, documentSource, checkpoint, err := s.documentsForSource(ctx, source)
	if err != nil {
		return map[string]any{"uploaded_documents": 0}, checkpoint, err
	}
	if len(documents) == 0 {
		stats := map[string]any{"uploaded_documents": 0, "fetched_documents": 0, "document_source": documentSource}
		if documentSource == "api" {
			stats["no_changes"] = true
			return stats, checkpoint, nil
		}
		return stats, checkpoint, errors.New("connector sync produced no documents")
	}

	forceFullSync := boolFromAny(source.Config["force_full_sync"], false)
	previousHashes := stringMapFromAny(nonNilMap(source.Checkpoint)["document_hashes"])
	nextHashes := copyStringMap(previousHashes)
	uploaded := 0
	skippedUnchanged := 0
	for i, doc := range documents {
		fileName := strings.TrimSpace(doc.FileName)
		if fileName == "" {
			fileName = fmt.Sprintf("%s-document-%d.txt", source.Type, i+1)
		}
		mimeType := strings.TrimSpace(doc.MimeType)
		if mimeType == "" {
			mimeType = "text/plain"
		}
		content := []byte(doc.Content)
		if len(content) == 0 {
			continue
		}
		sourceID := firstNonEmpty(doc.SourceID, fileName)
		hash := contentHash(content)
		if !forceFullSync && previousHashes[sourceID] == hash {
			nextHashes[sourceID] = hash
			skippedUnchanged++
			continue
		}
		if _, err := s.rag.UploadDocument(ctx, rag.Access{APIKeyID: access.APIKeyID, IsAdmin: access.IsAdmin}, source.TargetCollectionID, rag.UploadDocumentInput{
			FileName:   fileName,
			MimeType:   mimeType,
			Bytes:      content,
			SourceType: source.Type,
			SourceID:   sourceID,
			SourceURI:  doc.SourceURI,
			Metadata: map[string]any{
				"connector_source_id": source.ID,
				"connector_type":      source.Type,
			},
		}); err != nil {
			return map[string]any{"uploaded_documents": uploaded}, checkpoint, err
		}
		nextHashes[sourceID] = hash
		uploaded++
	}
	if uploaded == 0 {
		stats := map[string]any{
			"uploaded_documents": uploaded,
			"fetched_documents":  len(documents),
			"document_source":    documentSource,
		}
		if skippedUnchanged > 0 {
			stats["skipped_unchanged"] = skippedUnchanged
			checkpoint = nonNilMap(checkpoint)
			checkpoint["document_hashes"] = nextHashes
			return stats, checkpoint, nil
		}
		return stats, checkpoint, errors.New("connector sync produced no non-empty content")
	}
	checkpoint = nonNilMap(checkpoint)
	checkpoint["document_hashes"] = nextHashes
	return map[string]any{
		"uploaded_documents": uploaded,
		"fetched_documents":  len(documents),
		"skipped_unchanged":  skippedUnchanged,
		"document_source":    documentSource,
	}, checkpoint, nil
}

type connectorDocument struct {
	FileName  string
	MimeType  string
	Content   string
	SourceID  string
	SourceURI string
}

func (s *Service) documentsForSource(ctx context.Context, source sqlite.ConnectorSource) ([]connectorDocument, string, map[string]any, error) {
	if documents := documentsFromConfig(source.Config); len(documents) > 0 {
		return documents, "config", nil, nil
	}
	documents, checkpoint, err := s.fetchAPIDocuments(ctx, source)
	if err != nil {
		return nil, "api", checkpoint, err
	}
	return documents, "api", checkpoint, nil
}

func documentsFromConfig(config map[string]any) []connectorDocument {
	rawDocs, ok := config["documents"].([]any)
	if !ok {
		if content := strings.TrimSpace(stringFromAny(config["content"])); content != "" {
			return []connectorDocument{{
				FileName:  strings.TrimSpace(stringFromAny(config["file_name"])),
				MimeType:  strings.TrimSpace(stringFromAny(config["mime_type"])),
				Content:   content,
				SourceID:  strings.TrimSpace(stringFromAny(config["source_id"])),
				SourceURI: strings.TrimSpace(stringFromAny(config["source_uri"])),
			}}
		}
		return nil
	}
	out := []connectorDocument{}
	for _, raw := range rawDocs {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, connectorDocument{
			FileName:  strings.TrimSpace(stringFromAny(item["file_name"])),
			MimeType:  strings.TrimSpace(stringFromAny(item["mime_type"])),
			Content:   stringFromAny(item["content"]),
			SourceID:  strings.TrimSpace(stringFromAny(item["source_id"])),
			SourceURI: strings.TrimSpace(stringFromAny(item["source_uri"])),
		})
	}
	return out
}

func (s *Service) fetchAPIDocuments(ctx context.Context, source sqlite.ConnectorSource) ([]connectorDocument, map[string]any, error) {
	switch source.Type {
	case SourceTypeGitHub:
		docs, err := s.fetchGitHubDocuments(ctx, source)
		return docs, nil, err
	case SourceTypeSlack:
		return s.fetchSlackDocuments(ctx, source)
	case SourceTypeNotion:
		docs, err := s.fetchNotionDocuments(ctx, source)
		return docs, nil, err
	case SourceTypeLinear:
		docs, err := s.fetchLinearDocuments(ctx, source)
		return docs, nil, err
	default:
		return nil, nil, errors.New("unsupported connector source type")
	}
}

func (s *Service) fetchGitHubDocuments(ctx context.Context, source sqlite.ConnectorSource) ([]connectorDocument, error) {
	owner := strings.TrimSpace(stringFromAny(source.Config["owner"]))
	repo := strings.TrimSpace(stringFromAny(source.Config["repo"]))
	if owner == "" || repo == "" {
		return nil, errors.New("github config.owner and config.repo are required")
	}
	paths := stringListFromAny(source.Config["paths"])
	if len(paths) == 0 {
		if path := strings.TrimSpace(stringFromAny(source.Config["path"])); path != "" {
			paths = []string{path}
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("github config.paths must include at least one file path")
	}
	baseURL := connectorBaseURL(source, "https://api.github.com")
	docs := make([]connectorDocument, 0, len(paths))
	for _, filePath := range paths {
		filePath = strings.Trim(strings.TrimSpace(filePath), "/")
		if filePath == "" {
			continue
		}
		endpoint := strings.TrimRight(baseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/contents/" + escapePathSegments(filePath)
		req, err := s.newConnectorRequest(ctx, source, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		var payload struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Type        string `json:"type"`
			Encoding    string `json:"encoding"`
			Content     string `json:"content"`
			HTMLURL     string `json:"html_url"`
			DownloadURL string `json:"download_url"`
		}
		if err := s.doJSON(req, &payload); err != nil {
			return nil, err
		}
		if payload.Type != "" && payload.Type != "file" {
			return nil, fmt.Errorf("github path %q is not a file", filePath)
		}
		content := strings.ReplaceAll(payload.Content, "\n", "")
		var decoded []byte
		if strings.EqualFold(payload.Encoding, "base64") {
			decoded, err = base64.StdEncoding.DecodeString(content)
			if err != nil {
				return nil, fmt.Errorf("decode github file %q: %w", filePath, err)
			}
		} else {
			decoded = []byte(payload.Content)
		}
		name := payload.Name
		if name == "" {
			name = filepath.Base(filePath)
		}
		docs = append(docs, connectorDocument{
			FileName:  name,
			MimeType:  mimeTypeForFile(name),
			Content:   string(decoded),
			SourceID:  "github:" + owner + "/" + repo + ":" + firstNonEmpty(payload.Path, filePath),
			SourceURI: firstNonEmpty(payload.HTMLURL, payload.DownloadURL),
		})
	}
	return docs, nil
}

func (s *Service) fetchSlackDocuments(ctx context.Context, source sqlite.ConnectorSource) ([]connectorDocument, map[string]any, error) {
	channelID := strings.TrimSpace(firstNonEmpty(stringFromAny(source.Config["channel_id"]), stringFromAny(source.Config["channel"])))
	if channelID == "" {
		return nil, nil, errors.New("slack config.channel_id is required")
	}
	limit := intFromAny(source.Config["limit"], 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	baseURL := connectorBaseURL(source, "https://slack.com/api")
	values := url.Values{}
	values.Set("channel", channelID)
	values.Set("limit", strconv.Itoa(limit))
	if latestTS := strings.TrimSpace(stringFromAny(nonNilMap(source.Checkpoint)["slack_latest_ts"])); latestTS != "" {
		values.Set("oldest", latestTS)
		values.Set("inclusive", "false")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/conversations.history?" + values.Encode()
	req, err := s.newConnectorRequest(ctx, source, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	var payload struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Messages []struct {
			User string `json:"user"`
			Text string `json:"text"`
			TS   string `json:"ts"`
		} `json:"messages"`
	}
	if err := s.doJSON(req, &payload); err != nil {
		return nil, nil, err
	}
	if !payload.OK {
		return nil, nil, fmt.Errorf("slack conversations.history failed: %s", firstNonEmpty(payload.Error, "unknown_error"))
	}
	var b strings.Builder
	latestTS := strings.TrimSpace(stringFromAny(nonNilMap(source.Checkpoint)["slack_latest_ts"]))
	for _, msg := range payload.Messages {
		latestTS = maxSlackTimestamp(latestTS, msg.TS)
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		_, _ = fmt.Fprintf(&b, "[%s %s] %s\n", msg.TS, msg.User, text)
	}
	checkpoint := map[string]any{}
	if latestTS != "" {
		checkpoint["slack_latest_ts"] = latestTS
	}
	content := strings.TrimSpace(b.String())
	if content == "" {
		return nil, checkpoint, nil
	}
	return []connectorDocument{{
		FileName:  "slack-" + channelID + ".txt",
		MimeType:  "text/plain",
		Content:   content,
		SourceID:  "slack:" + channelID,
		SourceURI: "slack://channel/" + channelID,
	}}, checkpoint, nil
}

func (s *Service) fetchNotionDocuments(ctx context.Context, source sqlite.ConnectorSource) ([]connectorDocument, error) {
	pageID := strings.TrimSpace(firstNonEmpty(stringFromAny(source.Config["page_id"]), stringFromAny(source.Config["block_id"])))
	if pageID == "" {
		return nil, errors.New("notion config.page_id is required")
	}
	baseURL := connectorBaseURL(source, "https://api.notion.com/v1")
	values := url.Values{}
	values.Set("page_size", strconv.Itoa(intFromAny(source.Config["page_size"], 100)))
	endpoint := strings.TrimRight(baseURL, "/") + "/blocks/" + url.PathEscape(pageID) + "/children?" + values.Encode()
	req, err := s.newConnectorRequest(ctx, source, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	var b strings.Builder
	for _, block := range payload.Results {
		if text := notionBlockText(block); text != "" {
			b.WriteString(text)
			b.WriteString("\n")
		}
	}
	return []connectorDocument{{
		FileName:  "notion-" + pageID + ".txt",
		MimeType:  "text/plain",
		Content:   strings.TrimSpace(b.String()),
		SourceID:  "notion:" + pageID,
		SourceURI: "notion://block/" + pageID,
	}}, nil
}

func (s *Service) fetchLinearDocuments(ctx context.Context, source sqlite.ConnectorSource) ([]connectorDocument, error) {
	limit := intFromAny(source.Config["limit"], 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := `query ConnectorIssues($first: Int!) { issues(first: $first) { nodes { identifier title description url } } }`
	variables := map[string]any{"first": limit}
	if teamID := strings.TrimSpace(stringFromAny(source.Config["team_id"])); teamID != "" {
		query = `query ConnectorIssues($first: Int!, $teamID: String!) { issues(first: $first, filter: { team: { id: { eq: $teamID } } }) { nodes { identifier title description url } } }`
		variables["teamID"] = teamID
	}
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal linear query: %w", err)
	}
	endpoint := connectorBaseURL(source, "https://api.linear.app/graphql")
	req, err := s.newConnectorRequest(ctx, source, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var payload struct {
		Data struct {
			Issues struct {
				Nodes []struct {
					Identifier  string `json:"identifier"`
					Title       string `json:"title"`
					Description string `json:"description"`
					URL         string `json:"url"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"data"`
		Errors []map[string]any `json:"errors"`
	}
	if err := s.doJSON(req, &payload); err != nil {
		return nil, err
	}
	if len(payload.Errors) > 0 {
		return nil, errors.New("linear graphql returned errors")
	}
	var b strings.Builder
	for _, issue := range payload.Data.Issues.Nodes {
		_, _ = fmt.Fprintf(&b, "%s %s\n%s\n%s\n\n", issue.Identifier, issue.Title, issue.Description, issue.URL)
	}
	return []connectorDocument{{
		FileName:  "linear-issues.txt",
		MimeType:  "text/plain",
		Content:   strings.TrimSpace(b.String()),
		SourceID:  "linear:issues",
		SourceURI: "linear://issues",
	}}, nil
}

func (s *Service) newConnectorRequest(ctx context.Context, source sqlite.ConnectorSource, method, endpoint string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build connector request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token, err := connectorToken(source); err != nil {
		return nil, err
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if source.Type == SourceTypeNotion {
		req.Header.Set("Notion-Version", firstNonEmpty(stringFromAny(source.Config["notion_version"]), "2022-06-28"))
	}
	return req, nil
}

func (s *Service) doJSON(req *http.Request, out any) error {
	resp, err := s.connectorHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("connector request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if readErr != nil {
		return fmt.Errorf("read connector response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("connector upstream returned status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode connector response: %w", err)
	}
	return nil
}

func normalizeSourceType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case SourceTypeGitHub:
		return SourceTypeGitHub, nil
	case SourceTypeSlack:
		return SourceTypeSlack, nil
	case SourceTypeNotion:
		return SourceTypeNotion, nil
	case SourceTypeLinear:
		return SourceTypeLinear, nil
	default:
		return "", errors.New("type must be one of: github, slack, notion, linear")
	}
}

func scheduledSourceDue(source sqlite.ConnectorSource, now time.Time) bool {
	if !source.IsEnabled {
		return false
	}
	intervalSeconds := intFromAny(firstNonNil(source.Config["sync_interval_seconds"], source.Config["schedule_interval_seconds"]), 0)
	if intervalSeconds <= 0 {
		return false
	}
	if source.LastSyncAt == nil || source.LastSyncAt.IsZero() {
		return true
	}
	return !source.LastSyncAt.Add(time.Duration(intervalSeconds) * time.Second).After(now)
}

func nonNilMap(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func stringListFromAny(v any) []string {
	switch raw := v.(type) {
	case []string:
		return raw
	case []any:
		out := []string{}
		for _, item := range raw {
			if value := strings.TrimSpace(stringFromAny(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	case string:
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		parts := strings.Split(raw, ",")
		out := []string{}
		for _, part := range parts {
			if value := strings.TrimSpace(part); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func intFromAny(v any, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err == nil {
			return i
		}
	}
	return fallback
}

func boolFromAny(v any, fallback bool) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(b))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func stringMapFromAny(v any) map[string]string {
	out := map[string]string{}
	switch raw := v.(type) {
	case map[string]string:
		for key, value := range raw {
			out[key] = value
		}
	case map[string]any:
		for key, value := range raw {
			if s := strings.TrimSpace(stringFromAny(value)); s != "" {
				out[key] = s
			}
		}
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

func maxSlackTimestamp(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	af, aErr := strconv.ParseFloat(a, 64)
	bf, bErr := strconv.ParseFloat(b, 64)
	if aErr == nil && bErr == nil {
		if bf > af {
			return b
		}
		return a
	}
	if b > a {
		return b
	}
	return a
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func connectorBaseURL(source sqlite.ConnectorSource, fallback string) string {
	return firstNonEmpty(stringFromAny(source.Config["base_url"]), fallback)
}

func connectorToken(source sqlite.ConnectorSource) (string, error) {
	if err := validateCredentialRef(source); err != nil {
		return "", err
	}
	ref := strings.TrimSpace(source.CredentialRef)
	if ref == "" {
		return "", nil
	}
	token := strings.TrimSpace(os.Getenv(ref))
	if token == "" && connectorRequiresCredential(source) {
		return "", fmt.Errorf("credential_ref %q environment variable is not set", ref)
	}
	return token, nil
}

func connectorRequiresCredential(source sqlite.ConnectorSource) bool {
	// Inline documents are stored locally and are not fetched with a bearer token.
	if len(documentsFromConfig(source.Config)) > 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(source.Config["base_url"])) != "" && strings.EqualFold(stringFromAny(source.Config["allow_unauthenticated"]), "true") {
		return false
	}
	switch source.Type {
	case SourceTypeSlack, SourceTypeNotion, SourceTypeLinear:
		return true
	default:
		return false
	}
}

var connectorCredentialRefPattern = regexp.MustCompile(`^CONNECTOR_(GITHUB|SLACK|NOTION|LINEAR)_[A-Z0-9_]+$`)

func refuseConnectorRedirect(*http.Request, []*http.Request) error {
	return errors.New("connector redirects are not followed")
}

func (s *Service) validateConnectorSource(ctx context.Context, source sqlite.ConnectorSource) error {
	if err := validateCredentialRef(source); err != nil {
		return err
	}
	return s.validateBaseURL(ctx, source)
}

func validateCredentialRef(source sqlite.ConnectorSource) error {
	ref := strings.TrimSpace(source.CredentialRef)
	if ref == "" {
		if connectorRequiresCredential(source) {
			return errors.New(source.Type + " credential_ref is required")
		}
		return nil
	}
	match := connectorCredentialRefPattern.FindStringSubmatch(ref)
	if match == nil || match[1] != credentialProviderForType(source.Type) {
		return fmt.Errorf("credential_ref %q is not allowed", ref)
	}
	return nil
}

func credentialProviderForType(sourceType string) string {
	switch sourceType {
	case SourceTypeGitHub:
		return "GITHUB"
	case SourceTypeSlack:
		return "SLACK"
	case SourceTypeNotion:
		return "NOTION"
	case SourceTypeLinear:
		return "LINEAR"
	default:
		return ""
	}
}

func (s *Service) validateBaseURL(ctx context.Context, source sqlite.ConnectorSource) error {
	raw := strings.TrimSpace(stringFromAny(source.Config["base_url"]))
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "#") {
		return errors.New("config.base_url must not include a fragment")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("config.base_url must be an http or https URL")
	}
	if parsed.User != nil {
		return errors.New("config.base_url must not include userinfo")
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	if host == "" {
		return errors.New("config.base_url host is required")
	}
	ips, err := connectorHostIPs(ctx, host)
	if err != nil {
		return fmt.Errorf("config.base_url must be a public http(s) origin: %w", err)
	}
	if s != nil && s.allowPrivateBaseURL {
		return nil
	}
	for _, ip := range ips {
		if disallowedConnectorIP(ip) {
			return errors.New("config.base_url must be a public http(s) origin")
		}
	}
	return nil
}

func connectorHostIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if len(addr.IP) == 0 {
			continue
		}
		ips = append(ips, addr.IP)
	}
	if len(ips) == 0 {
		return nil, errors.New("host did not resolve")
	}
	return ips, nil
}

func disallowedConnectorIP(ip net.IP) bool {
	return len(ip) == 0 || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func (s *Service) connectorHTTPClient() *http.Client {
	if s != nil && s.httpClient != nil {
		return s.httpClient
	}
	return &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: refuseConnectorRedirect,
	}
}

func escapePathSegments(raw string) string {
	parts := strings.Split(raw, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func mimeTypeForFile(name string) string {
	if mimeType := mime.TypeByExtension(filepath.Ext(name)); mimeType != "" {
		return mimeType
	}
	return "text/plain"
}

func notionBlockText(block map[string]any) string {
	blockType := strings.TrimSpace(stringFromAny(block["type"]))
	if blockType == "" {
		return ""
	}
	payload, _ := block[blockType].(map[string]any)
	if len(payload) == 0 {
		return strings.TrimSpace(stringFromAny(block["plain_text"]))
	}
	if title := strings.TrimSpace(stringFromAny(payload["title"])); title != "" {
		return title
	}
	richText, _ := payload["rich_text"].([]any)
	parts := []string{}
	for _, raw := range richText {
		item, _ := raw.(map[string]any)
		if text := strings.TrimSpace(stringFromAny(item["plain_text"])); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}
