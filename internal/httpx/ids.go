package httpx

import (
	"crypto/rand"
	"encoding/base64"
)

func NewID(prefix string) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buf)
}
