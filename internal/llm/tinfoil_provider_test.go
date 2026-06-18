package llm

import (
	"errors"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	tinfoil "github.com/tinfoilsh/tinfoil-go"
)

func TestNewTinfoilProviderDefersExplicitClientWhenVerifyAtStartDisabled(t *testing.T) {
	original := newTinfoilClientWithParams
	t.Cleanup(func() {
		newTinfoilClientWithParams = original
	})

	called := false
	newTinfoilClientWithParams = func(_ string, _ string, _ ...option.RequestOption) (*tinfoil.Client, error) {
		called = true
		return nil, errors.New("client construction should be deferred")
	}

	provider, err := NewTinfoilProvider(
		"test-api-key",
		"example.invalid",
		"tinfoilsh/confidential-model-router",
		time.Second,
		false,
		false,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("NewTinfoilProvider() error = %v", err)
	}
	if called {
		t.Fatal("NewTinfoilProvider() constructed the Tinfoil client before verification was requested")
	}

	status := provider.AttestationStatus()
	if status.Verified {
		t.Fatal("AttestationStatus().Verified = true before verification")
	}
	if status.Enclave != "example.invalid" {
		t.Fatalf("AttestationStatus().Enclave = %q, want example.invalid", status.Enclave)
	}
	if status.Repo != "tinfoilsh/confidential-model-router" {
		t.Fatalf("AttestationStatus().Repo = %q, want tinfoilsh/confidential-model-router", status.Repo)
	}
}

func TestNewTinfoilProviderDefersDefaultClientWhenVerifyAtStartDisabled(t *testing.T) {
	original := newTinfoilClient
	t.Cleanup(func() {
		newTinfoilClient = original
	})

	called := false
	newTinfoilClient = func(_ ...option.RequestOption) (*tinfoil.Client, error) {
		called = true
		return nil, errors.New("default client construction should be deferred")
	}

	provider, err := NewTinfoilProvider(
		"test-api-key",
		"",
		"",
		time.Second,
		false,
		false,
		time.Minute,
	)
	if err != nil {
		t.Fatalf("NewTinfoilProvider() error = %v", err)
	}
	if called {
		t.Fatal("NewTinfoilProvider() constructed the default Tinfoil client before verification was requested")
	}
	if provider.AttestationStatus().Verified {
		t.Fatal("AttestationStatus().Verified = true before verification")
	}
}
