package httpx

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"enclava-go/internal/auth"
)

func (a *App) EnforceBudget(ctx context.Context, principal auth.Principal) error {
	if principal.IsUnlimited {
		return nil
	}
	hasAnyLimit := principal.BudgetLimitTokens != nil || principal.BudgetLimitCents != nil
	if !hasAnyLimit {
		return nil
	}
	if a.keysRepo == nil || a.usageRepo == nil {
		return nil
	}

	period := principal.BudgetPeriod
	if period == "" {
		period = "total"
	}

	var usedTokens, usedCents int64
	if period == "monthly" {
		now := time.Now().UTC()
		since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		toks, cents, err := a.usageRepo.SumSince(ctx, principal.APIKeyID, since)
		if err != nil {
			return err
		}
		usedTokens, usedCents = toks, cents
	} else {
		key, err := a.keysRepo.GetByID(ctx, principal.APIKeyID)
		if err != nil {
			return err
		}
		usedTokens, usedCents = key.TotalTokens, key.TotalCostCents
	}

	if principal.BudgetLimitTokens != nil && usedTokens >= *principal.BudgetLimitTokens {
		return errBudgetExceeded("token budget exceeded")
	}
	if principal.BudgetLimitCents != nil && usedCents >= *principal.BudgetLimitCents {
		return errBudgetExceeded("cost budget exceeded")
	}
	return nil
}

type budgetExceededError struct {
	msg string
}

func (e budgetExceededError) Error() string { return e.msg }

func errBudgetExceeded(msg string) error {
	return budgetExceededError{msg: msg}
}

func (a *App) WriteBudgetError(w http.ResponseWriter, err error) {
	var be budgetExceededError
	if errors.As(err, &be) {
		WriteOpenAIError(w, http.StatusTooManyRequests, be.Error(), "rate_limit_error", "budget_exceeded")
		return
	}
	log.Printf("budget check failed: %v", err)
	WriteOpenAIError(w, http.StatusInternalServerError, "internal server error", "server_error", "budget_check_failed")
}
