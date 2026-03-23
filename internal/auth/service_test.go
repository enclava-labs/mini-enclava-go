package auth

import (
	"context"
	"net/http/httptest"
	"testing"
)

type fakeRepo struct{}

func (f fakeRepo) FindCandidatesByPrefix(ctx context.Context, prefix string) ([]APIKeyRecord, error) {
	_, hash, usedAlgo, err := HashForStorage("enc_test_key_123456", HashAlgoSHA256, "")
	if err != nil {
		return nil, err
	}
	return []APIKeyRecord{{
		ID:          1,
		Name:        "test",
		KeyHash:     hash,
		KeyHashAlgo: usedAlgo,
		KeyPrefix:   "enc_test_",
		IsActive:    true,
		Scopes:      []string{"chat.completions"},
	}}, nil
}

func (f fakeRepo) TouchUsage(ctx context.Context, id int64) error { return nil }

func TestParseAPIKey(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer abc123")
	if got := ParseAPIKey(r); got != "abc123" {
		t.Fatalf("expected abc123, got %s", got)
	}
}

func TestAuthenticate(t *testing.T) {
	svc := NewService(fakeRepo{}, "")
	principal, err := svc.Authenticate(context.Background(), "enc_test_key_123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !HasScope(principal, "chat.completions") {
		t.Fatalf("expected scope")
	}
}
