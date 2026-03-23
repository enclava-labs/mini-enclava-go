package llm

import (
	"context"
	"io"
	"time"
)

type UpstreamError struct {
	StatusCode int
	Body       []byte
	Message    string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "upstream provider error"
}

type Provider interface {
	ListModels(ctx context.Context) (ModelsResponse, error)
	GetModel(ctx context.Context, modelID string) (Model, error)
	CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error)
	CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error)
	CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error)
}

type AttestationStatus struct {
	Enabled            bool      `json:"enabled"`
	Verified           bool      `json:"verified"`
	Provider           string    `json:"provider"`
	Enclave            string    `json:"enclave,omitempty"`
	Repo               string    `json:"repo,omitempty"`
	Digest             string    `json:"digest,omitempty"`
	CodeFingerprint    string    `json:"code_fingerprint,omitempty"`
	EnclaveFingerprint string    `json:"enclave_fingerprint,omitempty"`
	VerifiedAt         time.Time `json:"verified_at,omitempty"`
	LastError          string    `json:"last_error,omitempty"`
}

type AttestationProvider interface {
	VerifyAttestation(ctx context.Context) (AttestationStatus, error)
	AttestationStatus() AttestationStatus
}
