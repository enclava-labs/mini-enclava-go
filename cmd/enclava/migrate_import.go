package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"enclava-go/internal/store/sqlite"
)

type importFileSpec struct {
	name     string
	table    string
	importer func(context.Context, *sql.Tx, map[string]any) (bool, error)
}

type importReport struct {
	DryRun bool               `json:"dry_run"`
	Input  string             `json:"input"`
	Files  []importFileReport `json:"files"`
}

type importFileReport struct {
	File     string `json:"file"`
	Table    string `json:"table"`
	Rows     int    `json:"rows"`
	Existing int    `json:"existing"`
	Skipped  bool   `json:"skipped"`
}

func runMigrateImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate-import", flag.ContinueOnError)
	inputDir := fs.String("input-dir", "", "directory containing JSONL export files")
	databaseURL := fs.String("database", getenvDefault("DATABASE_URL", "./enclava.db"), "SQLite database path")
	dryRun := fs.Bool("dry-run", false, "validate JSONL files without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*inputDir) == "" {
		return errors.New("-input-dir is required")
	}
	report, err := migrateImport(ctx, strings.TrimSpace(*inputDir), strings.TrimSpace(*databaseURL), *dryRun)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	log.Printf("migrate-import report:\n%s", out)
	return nil
}

func migrateImport(ctx context.Context, inputDir, databaseURL string, dryRun bool) (importReport, error) {
	specs := []importFileSpec{
		{name: "users.jsonl", table: "users", importer: importUser},
		{name: "roles.jsonl", table: "roles", importer: importRole},
		{name: "api_keys.jsonl", table: "api_keys", importer: importAPIKey},
		{name: "budgets.jsonl", table: "budgets", importer: importBudget},
		{name: "provider_pricing.jsonl", table: "provider_pricing", importer: importProviderPricing},
		{name: "usage_records.jsonl", table: "usage_records", importer: importUsageRecord},
		{name: "extract_templates.jsonl", table: "extract_templates", importer: importExtractTemplate},
		{name: "prompt_templates.jsonl", table: "prompt_templates", importer: importPromptTemplate},
		{name: "chatbots.jsonl", table: "chatbot_instances", importer: importChatbot},
		{name: "chatbot_conversations.jsonl", table: "chatbot_conversations", importer: importChatbotConversation},
		{name: "rag_collections.jsonl", table: "rag_collections", importer: importRAGCollection},
		{name: "rag_documents.jsonl", table: "rag_documents", importer: importRAGDocument},
		{name: "connector_sources.jsonl", table: "connector_sources", importer: importConnectorSource},
	}
	report := importReport{DryRun: dryRun, Input: inputDir}

	var db *sqlite.DB
	var tx *sql.Tx
	var err error
	if !dryRun {
		db, err = sqlite.Open(ctx, databaseURL)
		if err != nil {
			return report, err
		}
		defer func() { _ = db.Close() }()
		tx, err = db.SQL.BeginTx(ctx, nil)
		if err != nil {
			return report, fmt.Errorf("begin import tx: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
	}

	for _, spec := range specs {
		path := filepath.Join(inputDir, spec.name)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				report.Files = append(report.Files, importFileReport{File: spec.name, Table: spec.table, Skipped: true})
				continue
			}
			return report, err
		}
		inserted, existing, err := importJSONLFile(ctx, tx, path, spec.importer, dryRun)
		if err != nil {
			return report, fmt.Errorf("import %s: %w", spec.name, err)
		}
		report.Files = append(report.Files, importFileReport{File: spec.name, Table: spec.table, Rows: inserted, Existing: existing})
	}

	if !dryRun {
		if err := recomputeImportedAPIKeyTotals(ctx, tx); err != nil {
			return report, err
		}
		if err := tx.Commit(); err != nil {
			return report, fmt.Errorf("commit import tx: %w", err)
		}
	}
	return report, nil
}

func importJSONLFile(ctx context.Context, tx *sql.Tx, path string, importer func(context.Context, *sql.Tx, map[string]any) (bool, error), dryRun bool) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	inserted := 0
	existing := 0
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return inserted, existing, fmt.Errorf("line %d invalid JSON: %w", lineNo, err)
		}
		if dryRun {
			tx = nil
		}
		ok, err := importer(ctx, tx, row)
		if err != nil {
			return inserted, existing, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if ok {
			inserted++
		} else {
			existing++
		}
	}
	if err := scanner.Err(); err != nil {
		return inserted, existing, err
	}
	return inserted, existing, nil
}

func execImport(ctx context.Context, tx *sql.Tx, query string, args ...any) (bool, error) {
	if tx == nil {
		return true, nil
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	switch affected {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected rows affected: %d", affected)
	}
}

func importUser(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustIntID(row, "id")
	if err != nil {
		return false, err
	}
	email, err := mustString(row, "email")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO users(id, email, display_name, password_hash, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, email, nullableStringValue(row["display_name"]),
		nullableStringValue(row["password_hash"]), boolInt(row["is_active"], true), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importRole(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustIntID(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO roles(id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, nullableStringValue(row["description"]), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importAPIKey(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustIntID(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	prefix, err := mustString(row, "key_prefix")
	if err != nil {
		return false, err
	}
	hash, err := mustString(row, "key_hash")
	if err != nil {
		return false, err
	}
	totalCostCents := int64FromAny(row["total_cost_cents"])
	return execImport(ctx, tx, `
		INSERT INTO api_keys(
			id, name, description, key_prefix, key_hash, key_hash_algo, is_active, is_deleted, deleted_at, expires_at,
			scopes, allowed_models, allowed_endpoints, allowed_extract_templates, allowed_chatbots, allowed_agents, allowed_ips,
			rate_limit_per_minute, rate_limit_per_hour, rate_limit_per_day,
			is_unlimited, budget_limit_tokens, budget_limit_cents, budget_period, tags,
			total_requests, total_tokens, total_cost_cents, total_cost_microcents, last_used_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, nullableStringValue(row["description"]),
		prefix, hash, defaultString(row["key_hash_algo"], "sha256"),
		boolInt(row["is_active"], true), boolInt(row["is_deleted"], false), nullableTimeString(row["deleted_at"]), nullableTimeString(row["expires_at"]),
		jsonString(row["scopes"], []string{}), jsonString(row["allowed_models"], []string{}), jsonString(row["allowed_endpoints"], []string{}),
		jsonString(row["allowed_extract_templates"], []string{}), jsonString(row["allowed_chatbots"], []string{}), jsonString(row["allowed_agents"], []string{}), jsonString(row["allowed_ips"], []string{}),
		nullableInt(row["rate_limit_per_minute"]), nullableInt(row["rate_limit_per_hour"]), nullableInt(row["rate_limit_per_day"]),
		boolInt(row["is_unlimited"], true), nullableInt(row["budget_limit_tokens"]), nullableInt(row["budget_limit_cents"]),
		nullableStringValue(row["budget_period"]), jsonString(row["tags"], []string{}),
		int64FromAny(row["total_requests"]), int64FromAny(row["total_tokens"]), totalCostCents, totalCostCents*1000000,
		nullableTimeString(row["last_used_at"]), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importBudget(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	subjectType, err := mustString(row, "subject_type")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO budgets(
			id, name, subject_type, subject_id, api_key_id, limit_tokens, limit_cents,
			period, starts_at, ends_at, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, subjectType, nullableStringValue(row["subject_id"]),
		nullableInt(row["api_key_id"]), nullableInt(row["limit_tokens"]), nullableInt(row["limit_cents"]),
		defaultString(row["period"], "monthly"), nullableTimeString(row["starts_at"]), nullableTimeString(row["ends_at"]),
		boolInt(row["is_active"], true), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importProviderPricing(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustIntID(row, "id")
	if err != nil {
		return false, err
	}
	providerID, err := mustString(row, "provider_id")
	if err != nil {
		return false, err
	}
	modelID, err := mustString(row, "model_id")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO provider_pricing(
			id, provider_id, model_id, currency,
			input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k,
			effective_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, providerID, modelID, defaultString(row["currency"], "USD"),
		floatFromAny(row["input_token_cents_per_1k"]), floatFromAny(row["output_token_cents_per_1k"]), floatFromAny(row["embedding_token_cents_per_1k"]),
		timeString(row["effective_at"]), timeString(row["created_at"]))
}

func importUsageRecord(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	endpoint, err := mustString(row, "endpoint")
	if err != nil {
		return false, err
	}
	costCents := int64FromAny(row["cost_cents"])
	return execImport(ctx, tx, `
		INSERT INTO usage_records(
			id, api_key_id, endpoint, model, chatbot_id, provider_id, provider_pricing_id, pricing_currency,
			input_token_cents_per_1k, output_token_cents_per_1k, embedding_token_cents_per_1k,
			prompt_tokens, completion_tokens, total_tokens, cost_cents, cost_microcents, status_code, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, int64FromAny(row["api_key_id"]), endpoint, nullableStringValue(row["model"]),
		nullableStringValue(row["chatbot_id"]), nullableStringValue(row["provider_id"]), nullableInt(row["provider_pricing_id"]),
		nullableStringValue(row["pricing_currency"]), floatFromAny(row["input_token_cents_per_1k"]), floatFromAny(row["output_token_cents_per_1k"]),
		floatFromAny(row["embedding_token_cents_per_1k"]), int64FromAny(row["prompt_tokens"]), int64FromAny(row["completion_tokens"]),
		int64FromAny(row["total_tokens"]), costCents, costCents*1000000, intFromAny(row["status_code"], 200), timeString(row["created_at"]))
}

func importExtractTemplate(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	systemPrompt, err := mustString(row, "system_prompt")
	if err != nil {
		return false, err
	}
	userPrompt, err := mustString(row, "user_prompt")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO extract_templates(
			id, name, description, system_prompt, user_prompt, context_schema, output_schema,
			model, is_default, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, nullableStringValue(row["description"]),
		systemPrompt, userPrompt,
		jsonString(firstPresent(row, "context_schema", "context_schema_json"), map[string]any{}),
		jsonString(firstPresent(row, "output_schema", "output_schema_json"), map[string]any{}),
		nullableStringValue(row["model"]), boolInt(row["is_default"], false), boolInt(row["is_active"], true),
		timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importPromptTemplate(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO prompt_templates(
			id, name, description, system_prompt, user_prompt, variables_schema,
			is_default, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, nullableStringValue(row["description"]),
		defaultString(row["system_prompt"], ""), defaultString(row["user_prompt"], "{{.user_input}}"),
		jsonString(firstPresent(row, "variables_schema", "variables_schema_json"), map[string]any{}),
		boolInt(row["is_default"], false), boolInt(row["is_active"], true), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importChatbot(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	model, err := mustString(row, "model")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO chatbot_instances(
			id, name, description, prompt_template_id, provider_id, model,
			rag_collection_ids, settings_json, external_id, is_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, nullableStringValue(row["description"]),
		nullableStringValue(row["prompt_template_id"]), nullableStringValue(row["provider_id"]), model,
		jsonString(row["rag_collection_ids"], []string{}), jsonString(firstPresent(row, "settings", "settings_json"), map[string]any{}),
		defaultString(row["external_id"], id), boolInt(row["is_enabled"], true), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importChatbotConversation(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	chatbotID, err := mustString(row, "chatbot_id")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO chatbot_conversations(
			id, chatbot_id, api_key_id, external_user_id, title, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, chatbotID, nullableInt(row["api_key_id"]),
		nullableStringValue(row["external_user_id"]), nullableStringValue(row["title"]),
		jsonString(firstPresent(row, "metadata", "metadata_json"), map[string]any{}), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importRAGCollection(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	embeddingModel, err := mustString(row, "embedding_model")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO rag_collections(
			id, name, description, owner_api_key_id, qdrant_collection,
			embedding_provider, embedding_model, chunk_size, chunk_overlap,
			acl_json, metadata_json, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, name, nullableStringValue(row["description"]),
		nullableInt(row["owner_api_key_id"]), defaultString(row["qdrant_collection"], "rag_"+id),
		nullableStringValue(row["embedding_provider"]), embeddingModel,
		intFromAny(row["chunk_size"], 1000), intFromAny(row["chunk_overlap"], 150),
		jsonString(firstPresent(row, "acl", "acl_json"), map[string]any{}), jsonString(firstPresent(row, "metadata", "metadata_json"), map[string]any{}),
		boolInt(row["is_active"], true), timeString(row["created_at"]), timeString(row["updated_at"]))
}

func importRAGDocument(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	collectionID, err := mustString(row, "collection_id")
	if err != nil {
		return false, err
	}
	fileName, err := mustString(row, "file_name")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO rag_documents(
			id, collection_id, source_type, source_id, source_uri, file_name, mime_type,
			status, text_hash, metadata_json, qdrant_point_count, error_message, created_at, updated_at, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, collectionID, defaultString(row["source_type"], "migration"),
		nullableStringValue(row["source_id"]), nullableStringValue(row["source_uri"]), fileName,
		defaultString(row["mime_type"], "application/octet-stream"), defaultString(row["status"], "ready"), nullableStringValue(row["text_hash"]),
		jsonString(firstPresent(row, "metadata", "metadata_json"), map[string]any{}), intFromAny(row["qdrant_point_count"], 0),
		nullableStringValue(row["error_message"]), timeString(row["created_at"]), timeString(row["updated_at"]), nullableTimeString(row["processed_at"]))
}

func importConnectorSource(ctx context.Context, tx *sql.Tx, row map[string]any) (bool, error) {
	id, err := mustString(row, "id")
	if err != nil {
		return false, err
	}
	sourceType, err := mustString(row, "type")
	if err != nil {
		return false, err
	}
	name, err := mustString(row, "name")
	if err != nil {
		return false, err
	}
	return execImport(ctx, tx, `
		INSERT INTO connector_sources(
			id, type, name, target_collection_id, config_json, credential_ref,
			checkpoint_json, is_enabled, created_at, updated_at, last_sync_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, sourceType, name, nullableStringValue(row["target_collection_id"]),
		jsonString(firstPresent(row, "config", "config_json"), map[string]any{}), nullableStringValue(row["credential_ref"]),
		jsonString(firstPresent(row, "checkpoint", "checkpoint_json"), map[string]any{}), boolInt(row["is_enabled"], true),
		timeString(row["created_at"]), timeString(row["updated_at"]), nullableTimeString(row["last_sync_at"]))
}

func recomputeImportedAPIKeyTotals(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET total_requests = (SELECT COUNT(*) FROM usage_records WHERE usage_records.api_key_id = api_keys.id),
		    total_tokens = (SELECT COALESCE(SUM(total_tokens), 0) FROM usage_records WHERE usage_records.api_key_id = api_keys.id),
		    total_cost_cents = (SELECT COALESCE(SUM(cost_cents), 0) FROM usage_records WHERE usage_records.api_key_id = api_keys.id),
		    total_cost_microcents = (SELECT COALESCE(SUM(cost_microcents), 0) FROM usage_records WHERE usage_records.api_key_id = api_keys.id),
		    updated_at = datetime('now')
	`)
	if err != nil {
		return fmt.Errorf("recompute api key totals: %w", err)
	}
	return nil
}

func mustString(row map[string]any, key string) (string, error) {
	value := requiredString(row, key)
	if value == "" {
		return "", fmt.Errorf("missing required value %q", key)
	}
	return value, nil
}

func mustIntID(row map[string]any, key string) (int64, error) {
	raw, ok := row[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("missing required value %q", key)
	}
	if text, isString := raw.(string); isString && strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("missing required value %q", key)
	}
	id := int64FromAny(raw)
	if id <= 0 {
		return 0, fmt.Errorf("missing required value %q", key)
	}
	return id, nil
}

func requiredString(row map[string]any, key string) string {
	return strings.TrimSpace(defaultString(row[key], ""))
}

func defaultString(v any, fallback string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return fallback
}

func nullableStringValue(v any) any {
	if s := defaultString(v, ""); s != "" {
		return s
	}
	return nil
}

func nullableTimeString(v any) any {
	if s := defaultString(v, ""); s != "" {
		return s
	}
	return nil
}

func timeString(v any) string {
	if s := defaultString(v, ""); s != "" {
		return s
	}
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func jsonString(v any, fallback any) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if v == nil {
		v = fallback
	}
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(fallback)
	}
	return string(b)
}

func firstPresent(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			return value
		}
	}
	return nil
}

func nullableInt(v any) any {
	if v == nil {
		return nil
	}
	switch raw := v.(type) {
	case string:
		if strings.TrimSpace(raw) == "" {
			return nil
		}
	}
	return int64FromAny(v)
}

func int64FromAny(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	default:
		return 0
	}
}

func intFromAny(v any, fallback int) int {
	if v == nil {
		return fallback
	}
	n := int64FromAny(v)
	if n == 0 {
		return fallback
	}
	return int(n)
}

func floatFromAny(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	default:
		return 0
	}
}

func boolInt(v any, fallback bool) int {
	value := fallback
	switch b := v.(type) {
	case bool:
		value = b
	case float64:
		value = b != 0
	case int:
		value = b != 0
	case string:
		if strings.TrimSpace(b) != "" {
			parsed, err := strconv.ParseBool(strings.TrimSpace(b))
			if err == nil {
				value = parsed
			}
		}
	}
	if value {
		return 1
	}
	return 0
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
