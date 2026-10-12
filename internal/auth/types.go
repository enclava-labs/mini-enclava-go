package auth

import "context"

type Principal struct {
	APIKeyID                int64
	Name                    string
	Scopes                  map[string]struct{}
	AllowedModels           map[string]struct{}
	AllowedExtractTemplates map[string]struct{}
	AllowedChatbots         map[string]struct{}
	AllowedAgents           map[string]struct{}
	AllowedEndpoints        map[string]struct{}
	AllowedIPs              map[string]struct{}
	RateLimitPerMinute      *int
	RateLimitPerHour        *int
	RateLimitPerDay         *int
	IsUnlimited             bool
	BudgetLimitTokens       *int64
	BudgetLimitCents        *int64
	BudgetPeriod            string
	RawAPIKeyPrefix         string
}

type contextKey string

const principalContextKey contextKey = "principal"

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey).(Principal)
	return p, ok
}

func HasScope(p Principal, scope string) bool {
	if _, ok := p.Scopes["*"]; ok {
		return true
	}
	_, ok := p.Scopes[scope]
	return ok
}

func ModelAllowed(p Principal, model string) bool {
	if len(p.AllowedModels) == 0 {
		return true
	}
	_, ok := p.AllowedModels[model]
	return ok
}

func ExtractTemplateAllowed(p Principal, templateID string) bool {
	if len(p.AllowedExtractTemplates) == 0 {
		return true
	}
	_, ok := p.AllowedExtractTemplates[templateID]
	return ok
}

func ChatbotAllowed(p Principal, chatbotIDs ...string) bool {
	if len(p.AllowedChatbots) == 0 {
		return true
	}
	if _, ok := p.AllowedChatbots["*"]; ok {
		return true
	}
	for _, id := range chatbotIDs {
		if _, ok := p.AllowedChatbots[id]; ok {
			return true
		}
	}
	return false
}

func AgentAllowed(p Principal, agentIDs ...string) bool {
	if len(p.AllowedAgents) == 0 {
		return true
	}
	if _, ok := p.AllowedAgents["*"]; ok {
		return true
	}
	for _, id := range agentIDs {
		if _, ok := p.AllowedAgents[id]; ok {
			return true
		}
	}
	return false
}
