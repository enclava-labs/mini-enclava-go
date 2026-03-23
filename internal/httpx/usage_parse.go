package httpx

import "encoding/json"

type usageParsed struct {
	Model            string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

func parseUsageFromResponse(body []byte) usageParsed {
	var env map[string]any
	if json.Unmarshal(body, &env) != nil {
		return usageParsed{}
	}
	out := usageParsed{}
	if model, ok := env["model"].(string); ok {
		out.Model = model
	}
	usageRaw, ok := env["usage"].(map[string]any)
	if !ok {
		return out
	}
	out.PromptTokens = int64(toFloat(usageRaw["prompt_tokens"]))
	out.CompletionTokens = int64(toFloat(usageRaw["completion_tokens"]))
	out.TotalTokens = int64(toFloat(usageRaw["total_tokens"]))
	return out
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
