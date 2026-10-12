package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProvidersRepoEnsureDefaultAndPromoteDefault(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "providers.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewProvidersRepo(db.SQL)
	if err := repo.EnsureDefault(ctx, ProviderConfig{
		ID:            "tinfoil",
		Type:          "tinfoil",
		Name:          "Tinfoil",
		BaseURL:       "https://inference.tinfoil.sh/v1",
		CredentialRef: "tinfoil_env",
		Config: map[string]any{
			"verify_at_start": true,
		},
		IsEnabled: true,
		IsDefault: true,
	}); err != nil {
		t.Fatalf("ensure default provider: %v", err)
	}

	if err := repo.Create(ctx, ProviderConfig{
		ID:            "private",
		Type:          "privatemode",
		Name:          "PrivateMode",
		BaseURL:       "https://privatemode.example/v1",
		CredentialRef: "privatemode_env",
		Config:        map[string]any{},
		IsEnabled:     true,
		IsDefault:     true,
	}); err != nil {
		t.Fatalf("create promoted provider: %v", err)
	}

	providers, err := repo.List(ctx, "", true)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	defaults := 0
	for _, provider := range providers {
		if provider.IsDefault {
			defaults++
			if provider.ID != "private" {
				t.Fatalf("unexpected default provider: %#v", provider)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("expected one default provider, got %d: %#v", defaults, providers)
	}

	private, err := repo.Get(ctx, "private")
	if err != nil {
		t.Fatalf("get private provider: %v", err)
	}
	private.IsEnabled = false
	if err := repo.Update(ctx, private); err != nil {
		t.Fatalf("update provider: %v", err)
	}
	enabled, err := repo.List(ctx, "", false)
	if err != nil {
		t.Fatalf("list enabled providers: %v", err)
	}
	if len(enabled) != 1 || enabled[0].ID != "tinfoil" {
		t.Fatalf("expected only tinfoil enabled, got %#v", enabled)
	}
}

func TestProvidersRepoEnsureDefaultLeavesEditedBaseURL(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "providers.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewProvidersRepo(db.SQL)
	seed := ProviderConfig{
		ID:            "tinfoil",
		Type:          "tinfoil",
		Name:          "Tinfoil",
		BaseURL:       "https://inference.tinfoil.sh/v1",
		CredentialRef: "tinfoil_env",
		Config: map[string]any{
			"verify_at_start": true,
		},
		IsEnabled: true,
		IsDefault: true,
	}
	if err := repo.EnsureDefault(ctx, seed); err != nil {
		t.Fatalf("ensure default provider: %v", err)
	}

	const edited = "https://operator.example/v1"
	if _, err := db.SQL.ExecContext(ctx, `UPDATE provider_configs SET base_url = ? WHERE id = ?`, edited, seed.ID); err != nil {
		t.Fatalf("edit base_url: %v", err)
	}

	seed.BaseURL = "https://boot.example/v1"
	if err := repo.EnsureDefault(ctx, seed); err != nil {
		t.Fatalf("ensure default provider again: %v", err)
	}
	got, err := repo.Get(ctx, seed.ID)
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if got.BaseURL != edited {
		t.Fatalf("expected edited base_url %q to remain, got %q", edited, got.BaseURL)
	}
}
