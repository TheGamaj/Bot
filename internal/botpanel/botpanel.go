// Package botpanel serves the Gamaj Bot Panel (is.0.0.1): the web management
// UI of Gamaj Bot. It is protected by the bot's admin Telegram ID login (the
// same admin_id from the configuration) and renders live sales data from the
// Gamaj API: plans, orders, buyer wallets and gateway status. The panel is
// read-only and stateless — every data call is proxied to the Gamaj API with
// the bot's dedicated API key.
package botpanel

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed index.html
var indexHTML string

// Version is the Gamaj Bot Panel release version.
const Version = "is.0.0.1"

// Handler serves the Bot Panel page on /panel and its JSON proxy on
// /panel/api/*. The login gate compares the submitted admin ID against the
// bot configuration's admin_id using constant-time comparison.
func Handler(adminID string, proxy http.Handler) http.Handler {
	mux := http.NewServeMux()
	panel := &panelServer{adminID: strings.TrimSpace(adminID), proxy: proxy}
	mux.Handle("/panel", http.HandlerFunc(panel.handlePage))
	mux.Handle("/panel/", http.HandlerFunc(panel.handle))
	return mux
}

type panelServer struct {
	adminID string
	proxy   http.Handler
}

func (p *panelServer) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/panel" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write([]byte(indexHTML))
}

func (p *panelServer) handle(w http.ResponseWriter, r *http.Request) {
	switch path := strings.TrimPrefix(r.URL.Path, "/panel/"); {
	case path == "api":
		http.NotFound(w, r)
	case strings.HasPrefix(path, "api/"):
		if !p.authorized(r) {
			http.Error(w, `{"detail":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		r.URL.Path = "/" + strings.TrimPrefix(path, "api/")
		p.proxy.ServeHTTP(w, r)
	default:
		http.Redirect(w, r, "/panel", http.StatusMovedPermanently)
	}
}

// authorized checks the panel session cookie set by the login call.
func (p *panelServer) authorized(r *http.Request) bool {
	cookie, err := r.Cookie("gamaj_bot_panel")
	if err != nil || cookie.Value == "" || len(cookie.Value) != len(p.adminID) {
		return false
	}
	return constantTimeEquals(cookie.Value, p.adminID)
}

func constantTimeEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
