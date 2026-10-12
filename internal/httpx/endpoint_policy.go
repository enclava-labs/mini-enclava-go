package httpx

import "strings"

func endpointAllowed(allowed map[string]struct{}, path string) bool {
	path = canonicalEndpointPath(path)
	if _, ok := allowed[path]; ok {
		return true
	}
	// Support simple prefix globs like "/api/v1/extract/*".
	for pattern := range allowed {
		pattern = canonicalEndpointPath(pattern)
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		if pattern == path {
			return true
		}
	}
	return false
}

func canonicalEndpointPath(path string) string {
	if path == "/v1" {
		return "/api/v1"
	}
	if strings.HasPrefix(path, "/v1/") {
		return "/api/v1" + strings.TrimPrefix(path, "/v1")
	}
	return path
}
