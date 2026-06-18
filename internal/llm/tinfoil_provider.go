package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3/option"
	tinfoil "github.com/tinfoilsh/tinfoil-go"
	"golang.org/x/sync/singleflight"
)

var (
	newTinfoilClient           = tinfoil.NewClient
	newTinfoilClientWithParams = tinfoil.NewClientWithParams
)

type TinfoilProvider struct {
	clientMu      sync.Mutex
	client        *tinfoil.Client
	apiKey        string
	enclave       string
	repo          string
	baseURL       string
	jsonClient    *http.Client
	streamClient  *http.Client
	timeout       time.Duration
	verifyPerCall bool
	verifyMaxAge  time.Duration
	verifyGroup   singleflight.Group

	mu     sync.RWMutex
	status AttestationStatus
}

func NewTinfoilProvider(
	apiKey string,
	enclave string,
	repo string,
	timeout time.Duration,
	verifyAtStart bool,
	verifyPerCall bool,
	verifyMaxAge time.Duration,
) (*TinfoilProvider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("TINFOIL_API_KEY is required for tinfoil provider")
	}

	hasEnclave := strings.TrimSpace(enclave) != ""
	hasRepo := strings.TrimSpace(repo) != ""
	if hasEnclave != hasRepo {
		return nil, fmt.Errorf("both TINFOIL_ENCLAVE and TINFOIL_REPO must be set together")
	}
	if !verifyPerCall && verifyMaxAge <= 0 {
		verifyMaxAge = 10 * time.Minute
		log.Printf("warning: TINFOIL_VERIFY_MAX_AGE is 0 with TINFOIL_VERIFY_PER_CALL=false; defaulting to %v", verifyMaxAge)
	}

	provider := &TinfoilProvider{
		apiKey:        apiKey,
		enclave:       strings.TrimSpace(enclave),
		repo:          strings.TrimSpace(repo),
		timeout:       timeout,
		verifyPerCall: verifyPerCall,
		verifyMaxAge:  verifyMaxAge,
		status: AttestationStatus{
			Enabled:  true,
			Verified: false,
			Provider: "tinfoil",
			Enclave:  strings.TrimSpace(enclave),
			Repo:     strings.TrimSpace(repo),
		},
	}

	if verifyAtStart {
		if _, err := provider.VerifyAttestation(context.Background()); err != nil {
			return nil, err
		}
	}

	return provider, nil
}

func (p *TinfoilProvider) AttestationStatus() AttestationStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *TinfoilProvider) VerifyAttestation(ctx context.Context) (AttestationStatus, error) {
	status := p.AttestationStatus()
	if status.Verified && !p.verifyPerCall && p.verifyMaxAge > 0 && time.Since(status.VerifiedAt) < p.verifyMaxAge {
		return status, nil
	}

	ch := p.verifyGroup.DoChan("verify", func() (any, error) {
		return p.verifyAttestationOnce()
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return p.AttestationStatus(), res.Err
		}
		return res.Val.(AttestationStatus), nil
	case <-ctx.Done():
		// Don't flip global attestation status on caller cancellation. Callers should treat ctx errors
		// as verification failures for this operation, but the last known good status may still hold.
		return p.AttestationStatus(), ctx.Err()
	}
}

func (p *TinfoilProvider) verifyAttestationOnce() (AttestationStatus, error) {
	client, err := p.ensureClient(nil)
	status := p.AttestationStatus()
	status.Enabled = true
	status.Provider = "tinfoil"
	status.Enclave = p.enclave
	status.Repo = p.repo

	if err != nil {
		status.Verified = false
		status.LastError = err.Error()
		p.setStatus(status)
		return status, err
	}

	groundTruth, err := client.Verify()
	if err != nil {
		status.Verified = false
		status.LastError = err.Error()
		p.setStatus(status)
		return status, fmt.Errorf("tinfoil attestation verification failed: %w", err)
	}

	status.Verified = true
	status.LastError = ""
	status.VerifiedAt = time.Now().UTC()
	status.Enclave = client.Enclave()
	status.Repo = client.Repo()
	if groundTruth != nil {
		status.Digest = groundTruth.Digest
		status.CodeFingerprint = groundTruth.CodeFingerprint
		status.EnclaveFingerprint = groundTruth.EnclaveFingerprint
	}
	p.setStatus(status)
	return status, nil
}

func (p *TinfoilProvider) ListModels(ctx context.Context) (ModelsResponse, error) {
	if err := p.ensureVerified(ctx); err != nil {
		return ModelsResponse{}, err
	}
	body, err := p.doJSON(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return ModelsResponse{}, err
	}
	var out ModelsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return ModelsResponse{}, fmt.Errorf("decode models response: %w", err)
	}
	return out, nil
}

func (p *TinfoilProvider) GetModel(ctx context.Context, modelID string) (Model, error) {
	if err := p.ensureVerified(ctx); err != nil {
		return Model{}, err
	}
	body, err := p.doJSON(ctx, http.MethodGet, "/models/"+url.PathEscape(modelID), nil)
	if err != nil {
		return Model{}, err
	}
	var out Model
	if err := json.Unmarshal(body, &out); err != nil {
		return Model{}, fmt.Errorf("decode model response: %w", err)
	}
	return out, nil
}

func (p *TinfoilProvider) CreateChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	if err := p.ensureVerified(ctx); err != nil {
		return nil, err
	}
	return p.doJSON(ctx, http.MethodPost, "/chat/completions", body)
}

func (p *TinfoilProvider) CreateEmbeddings(ctx context.Context, body []byte) ([]byte, error) {
	if err := p.ensureVerified(ctx); err != nil {
		return nil, err
	}
	return p.doJSON(ctx, http.MethodPost, "/embeddings", body)
}

func (p *TinfoilProvider) CreateChatCompletionStream(ctx context.Context, body []byte) (io.ReadCloser, error) {
	if err := p.ensureVerified(ctx); err != nil {
		return nil, err
	}
	_, streamClient, err := p.requestClients()
	if err != nil {
		return nil, err
	}
	requestURL := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream chat stream request failed: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer func() {
			_ = resp.Body.Close()
		}()
		upstreamBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: upstreamBody}
	}
	return resp.Body, nil
}

func (p *TinfoilProvider) ensureVerified(ctx context.Context) error {
	status := p.AttestationStatus()
	isStale := p.verifyMaxAge > 0 && status.Verified && time.Since(status.VerifiedAt) >= p.verifyMaxAge
	if p.verifyPerCall || !status.Verified || isStale {
		if _, err := p.VerifyAttestation(ctx); err != nil {
			return err
		}
	}
	if !p.AttestationStatus().Verified {
		return fmt.Errorf("tinfoil attestation not verified")
	}
	return nil
}

func (p *TinfoilProvider) doJSON(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	jsonClient, _, err := p.requestClients()
	if err != nil {
		return nil, err
	}
	requestURL := p.baseURL + path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := jsonClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: respBody}
	}

	return respBody, nil
}

func (p *TinfoilProvider) setStatus(status AttestationStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

func (p *TinfoilProvider) ensureClient(opts []option.RequestOption) (*tinfoil.Client, error) {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	if p.client != nil {
		return p.client, nil
	}
	if opts == nil {
		opts = []option.RequestOption{option.WithAPIKey(p.apiKey)}
	}

	var (
		tinfoilClient *tinfoil.Client
		err           error
	)
	if p.enclave != "" {
		tinfoilClient, err = newTinfoilClientWithParams(p.enclave, p.repo, opts...)
	} else {
		tinfoilClient, err = newTinfoilClient(opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("create tinfoil client: %w", err)
	}

	httpClient := tinfoilClient.HTTPClient()
	if p.timeout > 0 {
		httpClient.Timeout = p.timeout
	}

	streamClient := *httpClient
	// Streaming client has no timeout: long-running SSE streams must not be killed.
	// Mitigation: Go's net/http cancels the request when the client disconnects (via r.Context()).
	// Known limitation: if the upstream hangs without the client disconnecting, the goroutine leaks.
	streamClient.Timeout = 0

	p.client = tinfoilClient
	p.enclave = tinfoilClient.Enclave()
	p.repo = tinfoilClient.Repo()
	p.baseURL = fmt.Sprintf("https://%s/v1", tinfoilClient.Enclave())
	p.jsonClient = httpClient
	p.streamClient = &streamClient

	status := p.AttestationStatus()
	status.Enclave = p.enclave
	status.Repo = p.repo
	p.setStatus(status)
	return p.client, nil
}

func (p *TinfoilProvider) requestClients() (*http.Client, *http.Client, error) {
	if _, err := p.ensureClient(nil); err != nil {
		return nil, nil, err
	}

	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	return p.jsonClient, p.streamClient, nil
}
