package modules

import (
	"net/http"

	"enclava-go/internal/httpx"
)

// Module is a compile-time module. This is not a plugin system.
// The intent is to keep platform concerns (auth, budgets, usage, OpenAI compatibility)
// separate from module-specific routes so the active module can be replaced later.
type Module interface {
	Name() string
	Register(mux *http.ServeMux, platform *httpx.App)
}
