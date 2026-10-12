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
	if responseRaw, ok := env["response"].(map[string]any); ok {
		if model, ok := responseRaw["model"].(string); ok && out.Model == "" {
			out.Model = model
		}
		env = responseRaw
	}
	if out.PromptTokens == 0 {
		out.PromptTokens = int64(toFloat(env["input_tokens"]))
	}
	if out.CompletionTokens == 0 {
		out.CompletionTokens = int64(toFloat(env["output_tokens"]))
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = int64(toFloat(env["total_tokens"]))
	}
	usageRaw, ok := env["usage"].(map[string]any)
	if !ok {
		if out.TotalTokens == 0 {
			out.TotalTokens = out.PromptTokens + out.CompletionTokens
		}
		return out
	}
	out.PromptTokens = int64(toFloat(usageRaw["prompt_tokens"]))
	out.CompletionTokens = int64(toFloat(usageRaw["completion_tokens"]))
	out.TotalTokens = int64(toFloat(usageRaw["total_tokens"]))
	if out.PromptTokens == 0 {
		out.PromptTokens = int64(toFloat(usageRaw["input_tokens"]))
	}
	if out.CompletionTokens == 0 {
		out.CompletionTokens = int64(toFloat(usageRaw["output_tokens"]))
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
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
