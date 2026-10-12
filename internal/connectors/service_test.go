package connectors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"enclava-go/internal/rag"
	"enclava-go/internal/store/sqlite"
)

type fakeRAGUploader struct {
	uploads []rag.UploadDocumentInput
}

func (f *fakeRAGUploader) UploadDocument(ctx context.Context, access rag.Access, collectionID string, input rag.UploadDocumentInput) (rag.UploadDocumentOutput, error) {
	f.uploads = append(f.uploads, input)
	return rag.UploadDocumentOutput{
		Document: sqlite.RAGDocument{
			ID:           "ragdoc_test",
			CollectionID: collectionID,
			Status:       rag.DocumentStatusReady,
			FileName:     input.FileName,
			MimeType:     input.MimeType,
		},
	}, nil
}

func newConnectorTestService(t *testing.T) (context.Context, *Service, *fakeRAGUploader) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "connectors.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.NewRAGRepo(db.SQL).CreateCollection(ctx, sqlite.RAGCollection{
		ID:               "ragcol_test",
		Name:             "Connector Target",
		QdrantCollection: "rag_ragcol_test",
		EmbeddingModel:   "embed-test",
		ChunkSize:        rag.DefaultChunkSize,
		ChunkOverlap:     rag.DefaultChunkOverlap,
		ACL:              map[string]any{"public": true},
		Metadata:         map[string]any{},
		IsActive:         true,
	}); err != nil {
		t.Fatalf("create target collection: %v", err)
	}
	uploader := &fakeRAGUploader{}
	return ctx, NewService(sqlite.NewConnectorsRepo(db.SQL), uploader), uploader
}

func TestSyncSourceUploadsConfiguredDocuments(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeSlack,
		Name:               "Slack Fixture",
		TargetCollectionID: "ragcol_test",
		Config: map[string]any{
			"documents": []any{
				map[string]any{
					"file_name":  "thread.txt",
					"mime_type":  "text/plain",
					"content":    "connector fixture phrase",
					"source_id":  "thread-1",
					"source_uri": "slack://thread-1",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	job, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID)
	if err != nil {
		t.Fatalf("sync source: %v", err)
	}
	if job.Status != JobStatusSucceeded {
		t.Fatalf("expected succeeded job, got %#v", job)
	}
	if got := int(job.Stats["uploaded_documents"].(float64)); got != 1 {
		t.Fatalf("expected uploaded count 1, got %d", got)
	}
	if len(uploader.uploads) != 1 {
		t.Fatalf("expected one upload, got %d", len(uploader.uploads))
	}
	if uploader.uploads[0].SourceType != SourceTypeSlack || uploader.uploads[0].SourceID != "thread-1" {
		t.Fatalf("unexpected upload metadata: %#v", uploader.uploads[0])
	}

	updated, err := svc.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updated.LastSyncAt == nil || updated.Checkpoint["last_sync_status"] != JobStatusSucceeded {
		t.Fatalf("source sync metadata not updated: %#v", updated)
	}
}

func TestSyncSourceSkipsUnchangedDocumentsUsingCheckpoint(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeGitHub,
		Name:               "Checkpoint Fixture",
		TargetCollectionID: "ragcol_test",
		Config: map[string]any{
			"documents": []any{
				map[string]any{
					"file_name": "checkpoint.txt",
					"content":   "unchanged connector checkpoint phrase",
					"source_id": "checkpoint-doc",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err != nil {
		t.Fatalf("first sync source: %v", err)
	}
	if len(uploader.uploads) != 1 {
		t.Fatalf("expected first upload, got %d", len(uploader.uploads))
	}

	job, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID)
	if err != nil {
		t.Fatalf("second sync source: %v", err)
	}
	if job.Status != JobStatusSucceeded {
		t.Fatalf("expected second sync to succeed, got %#v", job)
	}
	if len(uploader.uploads) != 1 {
		t.Fatalf("expected unchanged document to be skipped, uploads=%d", len(uploader.uploads))
	}
	if got := int(job.Stats["skipped_unchanged"].(float64)); got != 1 {
		t.Fatalf("expected skipped unchanged count 1, got %d", got)
	}
	updated, err := svc.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("get updated source: %v", err)
	}
	hashes, ok := updated.Checkpoint["document_hashes"].(map[string]any)
	if !ok || hashes["checkpoint-doc"] == "" {
		t.Fatalf("expected checkpoint document hash, got %#v", updated.Checkpoint)
	}
}

func TestRunScheduledSyncsOnlyRunsDueSources(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeGitHub,
		Name:               "Scheduled Fixture",
		TargetCollectionID: "ragcol_test",
		Config: map[string]any{
			"sync_interval_seconds": 3600,
			"force_full_sync":       true,
			"documents": []any{
				map[string]any{"file_name": "scheduled.txt", "content": "scheduled connector phrase"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	now := time.Now().UTC()
	started, err := svc.RunScheduledSyncs(ctx, Access{APIKeyID: 42, IsAdmin: true}, now)
	if err != nil {
		t.Fatalf("run scheduled syncs: %v", err)
	}
	if started != 1 || len(uploader.uploads) != 1 {
		t.Fatalf("expected first scheduled sync, started=%d uploads=%d", started, len(uploader.uploads))
	}
	started, err = svc.RunScheduledSyncs(ctx, Access{APIKeyID: 42, IsAdmin: true}, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("run scheduled syncs second pass: %v", err)
	}
	if started != 0 || len(uploader.uploads) != 1 {
		t.Fatalf("expected source not due, started=%d uploads=%d", started, len(uploader.uploads))
	}
	started, err = svc.RunScheduledSyncs(ctx, Access{APIKeyID: 42, IsAdmin: true}, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("run scheduled syncs third pass: %v", err)
	}
	if started != 1 || len(uploader.uploads) != 2 {
		t.Fatalf("expected source due again, source=%s started=%d uploads=%d", source.ID, started, len(uploader.uploads))
	}
}

func TestSyncGitHubSourceFetchesFiles(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	svc.setAllowPrivateBaseURL(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/docs/contents/README.md" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeTestJSON(t, w, map[string]any{
			"name":     "README.md",
			"path":     "README.md",
			"type":     "file",
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString([]byte("github connector maple")),
			"html_url": "https://github.example/acme/docs/README.md",
		})
	}))
	defer server.Close()

	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeGitHub,
		Name:               "GitHub Docs",
		TargetCollectionID: "ragcol_test",
		Config: map[string]any{
			"base_url": server.URL,
			"owner":    "acme",
			"repo":     "docs",
			"paths":    []any{"README.md"},
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err != nil {
		t.Fatalf("sync source: %v", err)
	}
	if len(uploader.uploads) != 1 || !strings.Contains(string(uploader.uploads[0].Bytes), "github connector maple") {
		t.Fatalf("unexpected github upload: %#v", uploader.uploads)
	}
	if uploader.uploads[0].SourceType != SourceTypeGitHub || uploader.uploads[0].SourceID != "github:acme/docs:README.md" {
		t.Fatalf("unexpected github source metadata: %#v", uploader.uploads[0])
	}
}

func TestSyncSlackSourceFetchesChannelHistory(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	svc.setAllowPrivateBaseURL(true)
	t.Setenv("CONNECTOR_SLACK_TOKEN", "slack-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer slack-token" {
			t.Fatalf("missing auth header")
		}
		if r.URL.Query().Get("channel") != "C123" {
			t.Fatalf("unexpected channel: %s", r.URL.RawQuery)
		}
		writeTestJSON(t, w, map[string]any{
			"ok": true,
			"messages": []any{
				map[string]any{"user": "U1", "text": "slack connector cedar", "ts": "1.0"},
			},
		})
	}))
	defer server.Close()

	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeSlack,
		Name:               "Slack Channel",
		TargetCollectionID: "ragcol_test",
		CredentialRef:      "CONNECTOR_SLACK_TOKEN",
		Config: map[string]any{
			"base_url":   server.URL,
			"channel_id": "C123",
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err != nil {
		t.Fatalf("sync source: %v", err)
	}
	if len(uploader.uploads) != 1 || !strings.Contains(string(uploader.uploads[0].Bytes), "slack connector cedar") {
		t.Fatalf("unexpected slack upload: %#v", uploader.uploads)
	}
}

func TestSyncSlackSourceUsesTimestampCheckpoint(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	svc.setAllowPrivateBaseURL(true)
	t.Setenv("CONNECTOR_SLACK_TOKEN", "slack-token")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			if r.URL.Query().Get("oldest") != "" {
				t.Fatalf("first request should not include oldest: %s", r.URL.RawQuery)
			}
			writeTestJSON(t, w, map[string]any{
				"ok": true,
				"messages": []any{
					map[string]any{"user": "U1", "text": "slack checkpoint first", "ts": "1000.000001"},
					map[string]any{"user": "U2", "text": "slack checkpoint second", "ts": "1000.000003"},
				},
			})
			return
		}
		if r.URL.Query().Get("oldest") != "1000.000003" || r.URL.Query().Get("inclusive") != "false" {
			t.Fatalf("second request missing checkpoint cursor: %s", r.URL.RawQuery)
		}
		writeTestJSON(t, w, map[string]any{
			"ok":       true,
			"messages": []any{},
		})
	}))
	defer server.Close()

	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeSlack,
		Name:               "Slack Checkpoint",
		TargetCollectionID: "ragcol_test",
		CredentialRef:      "CONNECTOR_SLACK_TOKEN",
		Config: map[string]any{
			"base_url":   server.URL,
			"channel_id": "C123",
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err != nil {
		t.Fatalf("first sync source: %v", err)
	}
	if len(uploader.uploads) != 1 {
		t.Fatalf("expected first slack upload, got %d", len(uploader.uploads))
	}
	job, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID)
	if err != nil {
		t.Fatalf("second sync source: %v", err)
	}
	if job.Status != JobStatusSucceeded {
		t.Fatalf("expected second slack sync to succeed, got %#v", job)
	}
	if len(uploader.uploads) != 1 {
		t.Fatalf("expected no new slack upload, got %d", len(uploader.uploads))
	}
	updated, err := svc.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("get updated source: %v", err)
	}
	if updated.Checkpoint["slack_latest_ts"] != "1000.000003" {
		t.Fatalf("expected slack timestamp checkpoint, got %#v", updated.Checkpoint)
	}
}

func TestSyncNotionSourceFetchesPageBlocks(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	svc.setAllowPrivateBaseURL(true)
	t.Setenv("CONNECTOR_NOTION_TOKEN", "notion-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Notion-Version") == "" {
			t.Fatalf("missing notion version header")
		}
		if r.URL.Path != "/blocks/page123/children" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeTestJSON(t, w, map[string]any{
			"results": []any{
				map[string]any{
					"type": "paragraph",
					"paragraph": map[string]any{
						"rich_text": []any{
							map[string]any{"plain_text": "notion connector pine"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeNotion,
		Name:               "Notion Page",
		TargetCollectionID: "ragcol_test",
		CredentialRef:      "CONNECTOR_NOTION_TOKEN",
		Config: map[string]any{
			"base_url": server.URL,
			"page_id":  "page123",
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err != nil {
		t.Fatalf("sync source: %v", err)
	}
	if len(uploader.uploads) != 1 || !strings.Contains(string(uploader.uploads[0].Bytes), "notion connector pine") {
		t.Fatalf("unexpected notion upload: %#v", uploader.uploads)
	}
}

func TestSyncLinearSourceFetchesIssues(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	svc.setAllowPrivateBaseURL(true)
	t.Setenv("CONNECTOR_LINEAR_TOKEN", "linear-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode linear request: %v", err)
		}
		if !strings.Contains(request["query"].(string), "issues") {
			t.Fatalf("unexpected linear query: %#v", request)
		}
		writeTestJSON(t, w, map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []any{
						map[string]any{
							"identifier":  "LIN-1",
							"title":       "Sync connector",
							"description": "linear connector aspen",
							"url":         "https://linear.example/LIN-1",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeLinear,
		Name:               "Linear Issues",
		TargetCollectionID: "ragcol_test",
		CredentialRef:      "CONNECTOR_LINEAR_TOKEN",
		Config: map[string]any{
			"base_url": server.URL,
			"team_id":  "team123",
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err != nil {
		t.Fatalf("sync source: %v", err)
	}
	if len(uploader.uploads) != 1 || !strings.Contains(string(uploader.uploads[0].Bytes), "linear connector aspen") {
		t.Fatalf("unexpected linear upload: %#v", uploader.uploads)
	}
}

func TestCreateSourceRejectsUnsafeCredentialRefs(t *testing.T) {
	ctx, svc, _ := newConnectorTestService(t)
	cases := []struct {
		name string
		ref  string
	}{
		{name: "admin password", ref: "ADMIN_PASSWORD"},
		{name: "privatemode key", ref: "PRIVATEMODE_API_KEY"},
		{name: "bootstrap key", ref: "BOOTSTRAP_API_KEY"},
		{name: "tinfoil key", ref: "TINFOIL_API_KEY"},
		{name: "cross type", ref: "CONNECTOR_GITHUB_TOKEN"},
		{name: "lowercase", ref: "connector_slack_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateSource(ctx, CreateSourceInput{
				Type:          SourceTypeSlack,
				Name:          "Unsafe credential",
				CredentialRef: tc.ref,
			})
			if err == nil {
				t.Fatalf("expected credential_ref %q to be rejected", tc.ref)
			}
		})
	}
	if _, err := svc.CreateSource(ctx, CreateSourceInput{
		Type: SourceTypeSlack,
		Name: "Missing credential",
	}); err == nil {
		t.Fatal("expected empty slack credential_ref to be rejected")
	}
	sources, err := svc.ListSources(ctx, "", 20, 0)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("rejected sources were stored: %#v", sources)
	}
}

func TestCreateSourceAcceptsMatchingCredentialRef(t *testing.T) {
	ctx, svc, _ := newConnectorTestService(t)
	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:          SourceTypeSlack,
		Name:          "Slack token",
		CredentialRef: "CONNECTOR_SLACK_TOKEN",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if source.CredentialRef != "CONNECTOR_SLACK_TOKEN" {
		t.Fatalf("unexpected credential_ref: %q", source.CredentialRef)
	}
	replacement := "ADMIN_PASSWORD"
	if _, err := svc.UpdateSource(ctx, source.ID, UpdateSourceInput{CredentialRef: &replacement}); err == nil {
		t.Fatal("expected update to reject ADMIN_PASSWORD")
	}
	stored, err := svc.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if stored.CredentialRef != "CONNECTOR_SLACK_TOKEN" {
		t.Fatalf("credential_ref changed after rejected update: %q", stored.CredentialRef)
	}
}

func TestCreateSourceRejectsPrivateBaseURL(t *testing.T) {
	ctx, svc, _ := newConnectorTestService(t)
	for _, rawURL := range []string{
		"http://169.254.169.254/",
		"http://10.1.2.3/",
		"http://127.0.0.1/",
		"http://192.168.1.20/",
		"http://[::1]/",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := svc.CreateSource(ctx, CreateSourceInput{
				Type: SourceTypeSlack,
				Name: "Private origin",
				Config: map[string]any{
					"base_url":              rawURL,
					"allow_unauthenticated": "true",
					"channel_id":            "C123",
				},
			})
			if err == nil {
				t.Fatalf("expected base_url %s to be rejected", rawURL)
			}
		})
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := svc.CreateSource(resolveCtx, CreateSourceInput{
		Type: SourceTypeGitHub,
		Name: "Unresolved origin",
		Config: map[string]any{
			"base_url": "http://connector-origin.invalid/v1",
			"owner":    "acme",
			"repo":     "docs",
			"paths":    []any{"README.md"},
		},
	}); err == nil {
		t.Fatal("expected unresolved base_url to be rejected")
	}
}

func TestConnectorRedirectDoesNotForwardAuthorization(t *testing.T) {
	ctx, svc, uploader := newConnectorTestService(t)
	svc.setAllowPrivateBaseURL(true)
	t.Setenv("CONNECTOR_SLACK_TOKEN", "super-secret")

	secondHits := 0
	secondAuth := ""
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		secondAuth = r.Header.Get("Authorization")
		writeTestJSON(t, w, map[string]any{
			"ok": true,
			"messages": []any{
				map[string]any{"user": "U1", "text": "should not be fetched", "ts": "1.0"},
			},
		})
	}))
	defer second.Close()

	firstHits := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		if got := r.Header.Get("Authorization"); got != "Bearer super-secret" {
			t.Errorf("first hop authorization = %q", got)
		}
		http.Redirect(w, r, second.URL+"/stolen", http.StatusFound)
	}))
	defer first.Close()

	source, err := svc.CreateSource(ctx, CreateSourceInput{
		Type:               SourceTypeSlack,
		Name:               "Redirect Slack",
		TargetCollectionID: "ragcol_test",
		CredentialRef:      "CONNECTOR_SLACK_TOKEN",
		Config: map[string]any{
			"base_url":   first.URL,
			"channel_id": "C123",
		},
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, source.ID); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("expected redirect to fail the sync, got %v", err)
	}
	if firstHits != 1 {
		t.Fatalf("expected one request to the redirecting server, got %d", firstHits)
	}
	if secondHits != 0 || secondAuth != "" {
		t.Fatalf("redirect was followed, hits=%d authorization=%q", secondHits, secondAuth)
	}
	if len(uploader.uploads) != 0 {
		t.Fatalf("expected no uploads after a refused redirect, got %d", len(uploader.uploads))
	}
}

func TestSyncImportedSourceDoesNotFetchInvalidConfig(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "connectors.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.NewRAGRepo(db.SQL).CreateCollection(ctx, sqlite.RAGCollection{
		ID:               "ragcol_test",
		Name:             "Connector Target",
		QdrantCollection: "rag_ragcol_test",
		EmbeddingModel:   "embed-test",
		ChunkSize:        rag.DefaultChunkSize,
		ChunkOverlap:     rag.DefaultChunkOverlap,
		ACL:              map[string]any{"public": true},
		Metadata:         map[string]any{},
		IsActive:         true,
	}); err != nil {
		t.Fatalf("create target collection: %v", err)
	}
	repo := sqlite.NewConnectorsRepo(db.SQL)
	uploader := &fakeRAGUploader{}
	svc := NewService(repo, uploader)

	t.Run("private base url", func(t *testing.T) {
		hits := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			writeTestJSON(t, w, map[string]any{"name": "README.md", "type": "file", "content": "nope"})
		}))
		defer server.Close()
		if err := repo.CreateSource(ctx, sqlite.ConnectorSource{
			ID:                 "connsrc_private_import",
			Type:               SourceTypeGitHub,
			Name:               "Imported private",
			TargetCollectionID: "ragcol_test",
			Config: map[string]any{
				"base_url": server.URL,
				"owner":    "acme",
				"repo":     "docs",
				"paths":    []any{"README.md"},
			},
			Checkpoint: map[string]any{},
			IsEnabled:  true,
		}); err != nil {
			t.Fatalf("import source: %v", err)
		}
		_, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, "connsrc_private_import")
		if err == nil || !strings.Contains(err.Error(), "base_url") {
			t.Fatalf("expected base_url validation error, got %v", err)
		}
		if hits != 0 {
			t.Fatalf("imported private base_url was requested %d times", hits)
		}
	})

	t.Run("process env credential", func(t *testing.T) {
		t.Setenv("ADMIN_PASSWORD", "super-secret-admin")
		svc.setAllowPrivateBaseURL(true)
		hits := 0
		seenAuth := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			seenAuth = r.Header.Get("Authorization")
			writeTestJSON(t, w, map[string]any{"ok": true, "messages": []any{}})
		}))
		defer server.Close()
		if err := repo.CreateSource(ctx, sqlite.ConnectorSource{
			ID:                 "connsrc_admin_import",
			Type:               SourceTypeSlack,
			Name:               "Imported admin",
			TargetCollectionID: "ragcol_test",
			CredentialRef:      "ADMIN_PASSWORD",
			Config: map[string]any{
				"base_url":   server.URL,
				"channel_id": "C123",
			},
			Checkpoint: map[string]any{},
			IsEnabled:  true,
		}); err != nil {
			t.Fatalf("import source: %v", err)
		}
		_, err := svc.SyncSource(ctx, Access{APIKeyID: 42}, "connsrc_admin_import")
		if err == nil || !strings.Contains(err.Error(), "credential_ref") {
			t.Fatalf("expected credential_ref validation error, got %v", err)
		}
		if hits != 0 || seenAuth != "" {
			t.Fatalf("imported credential was sent, hits=%d authorization=%q", hits, seenAuth)
		}
		if len(uploader.uploads) != 0 {
			t.Fatalf("expected no uploads, got %d", len(uploader.uploads))
		}
	})
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
