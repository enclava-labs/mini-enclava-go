package sqlite

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, table := range []string{
		"api_keys",
		"usage_records",
		"provider_pricing",
		"provider_configs",
		"billing_audit_log",
		"budgets",
		"rag_collections",
		"rag_documents",
		"rag_document_blobs",
		"prompt_templates",
		"chatbot_instances",
		"connector_sources",
		"connector_sync_jobs",
	} {
		if !tableExists(t, db.SQL, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
	if !indexExists(t, db.SQL, "idx_rag_documents_collection_status") {
		t.Fatalf("expected RAG hot-path index")
	}
	if !indexExists(t, db.SQL, "idx_connector_sync_jobs_source_created") {
		t.Fatalf("expected connector sync hot-path index")
	}
	if !indexExists(t, db.SQL, "idx_usage_records_chatbot_created_at") {
		t.Fatalf("expected chatbot usage attribution index")
	}
	if !indexExists(t, db.SQL, "idx_usage_records_provider_model_created_at") {
		t.Fatalf("expected usage provider/model pricing index")
	}
	if !indexExists(t, db.SQL, "idx_provider_configs_type_enabled") {
		t.Fatalf("expected provider config hot-path index")
	}
	if got := foreignKeyEnabled(t, db.SQL); got != 1 {
		t.Fatalf("expected foreign_keys pragma to be enabled, got %d", got)
	}
	if fkCount := foreignKeyCount(t, db.SQL, "rag_documents"); fkCount == 0 {
		t.Fatalf("expected rag_documents foreign key")
	}
	if !columnExists(t, db.SQL, "extract_templates", "output_schema") {
		t.Fatalf("expected extract_templates.output_schema column")
	}
	if !columnExists(t, db.SQL, "api_keys", "allowed_agents") {
		t.Fatalf("expected api_keys.allowed_agents column")
	}
}

func TestOpenMigratesCurrentMiniSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "current-mini.db")
	if err := seedMigrationsThrough(ctx, path, "005_usage_records.sql"); err != nil {
		t.Fatalf("seed current mini schema: %v", err)
	}

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() { _ = db.Close() }()

	if !tableExists(t, db.SQL, "rag_collections") {
		t.Fatalf("expected rag_collections table after upgrade")
	}
	if !tableExists(t, db.SQL, "chatbot_messages") {
		t.Fatalf("expected chatbot_messages table after upgrade")
	}
	if !migrationRecorded(t, db.SQL, "006_foundation_schema.sql") {
		t.Fatalf("expected 006_foundation_schema.sql migration record")
	}
	if !migrationRecorded(t, db.SQL, "013_api_key_allowed_agents.sql") {
		t.Fatalf("expected 013_api_key_allowed_agents.sql migration record")
	}
	if !columnExists(t, db.SQL, "api_keys", "allowed_agents") {
		t.Fatalf("expected api_keys.allowed_agents column after upgrade")
	}
}

func seedMigrationsThrough(ctx context.Context, path, through string) error {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES (?)", name); err != nil {
			return err
		}
		if name == through {
			break
		}
	}
	return nil
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'table'
		  AND name = ?
	`, table).Scan(&count); err != nil {
		t.Fatalf("table exists query failed: %v", err)
	}
	return count == 1
}

func indexExists(t *testing.T, db *sql.DB, index string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'index'
		  AND name = ?
	`, index).Scan(&count); err != nil {
		t.Fatalf("index exists query failed: %v", err)
	}
	return count == 1
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("table info query failed: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info failed: %v", err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table info rows failed: %v", err)
	}
	return false
}

func migrationRecorded(t *testing.T, db *sql.DB, version string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
		t.Fatalf("migration query failed: %v", err)
	}
	return count == 1
}

func foreignKeyEnabled(t *testing.T, db *sql.DB) int {
	t.Helper()
	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("foreign_keys pragma failed: %v", err)
	}
	return enabled
}

func foreignKeyCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	rows, err := db.Query("PRAGMA foreign_key_list(" + table + ")")
	if err != nil {
		t.Fatalf("foreign key list failed: %v", err)
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign key list rows failed: %v", err)
	}
	return count
}
