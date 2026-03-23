package httpx

import "strings"

func endpointAllowed(allowed map[string]struct{}, path string) bool {
	if _, ok := allowed[path]; ok {
		return true
	}
	// Support simple prefix globs like "/api/v1/extract/*".
	for pattern := range allowed {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}
