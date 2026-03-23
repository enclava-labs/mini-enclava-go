package httpx

import (
	"net"
	"net/http"
	"strings"
)

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ip := strings.TrimSpace(parts[0])
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
		xrip := strings.TrimSpace(r.Header.Get("X-Real-IP"))
		if net.ParseIP(xrip) != nil {
			return xrip
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	if net.ParseIP(r.RemoteAddr) != nil {
		return r.RemoteAddr
	}
	return ""
}
