package httpx

import (
	"context"

	"enclava-go/internal/llm"
	"enclava-go/internal/store/sqlite"
)

func (a *App) Provider() llm.Provider {
	return a.provider
}

func (a *App) ProviderReady() bool {
	if a.prober == nil {
		return true
	}
	return a.prober.Ready()
}

func (a *App) ProviderSnapshot() any {
	if a.prober == nil {
		return nil
	}
	return a.prober.Snapshot()
}

func (a *App) RecordUsage(ctx context.Context, rec sqlite.UsageRecord) {
	if a.usageRepo == nil {
		return
	}
	_ = a.usageRepo.Record(ctx, rec)
}
