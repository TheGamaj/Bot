// Package botpanel serves the Gamaj Bot Panel (is.0.0.1): the web management
// UI of Gamaj Bot. It is protected by the bot's admin Telegram ID login (the
// same admin_id from the configuration) and renders live sales data from the
// Gamaj API: plans, orders, buyer wallets and gateway status. The panel is
// read-only and stateless — every data call is proxied to the Gamaj API with
// the bot's dedicated API key.
package botpanel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

//go:embed index.html
var indexHTML string

// Version is the Gamaj Bot Panel release version.
const Version = "is.0.0.1"

const (
	// sessionCookie is the panel session cookie. Its value is an HMAC over the
	// configured admin ID, so the value alone cannot be forged: knowing the
	// numeric admin ID is no longer enough to mint a session.
	sessionCookie = "gamaj_bot_panel"
	// adminHeader carries the candidate admin ID during login.
	adminHeader = "X-Panel-Admin-Id"
	// proxyPrefix is the only part of the Gamaj API this panel may reach. The
	// bot's API key is attached to every proxied call, so exposing anything
	// else would let a browser borrow the bot's panel-wide authority.
	proxyPrefix = "api/bot/"
)

// Handler serves the Bot Panel page on /panel and its JSON proxy on
// /panel/api/*. Login compares the submitted admin ID against the bot
// configuration's admin_id using constant-time comparison and then issues a
// signed session cookie.
func Handler(adminID string, proxy http.Handler) http.Handler {
	sessionKey := make([]byte, 32)
	if _, err := rand.Read(sessionKey); err != nil {
		// A predictable key would make every session forgeable, so refuse to
		// serve the panel at all rather than fall back to a weak secret.
		panic("botpanel: cannot seed the session key: " + err.Error())
	}
	mux := http.NewServeMux()
	panel := &panelServer{
		adminID:    strings.TrimSpace(adminID),
		proxy:      proxy,
		sessionKey: sessionKey,
	}
	mux.Handle("/panel", http.HandlerFunc(panel.handlePage))
	mux.Handle("/panel/", http.HandlerFunc(panel.handle))
	return mux
}

type panelServer struct {
	adminID    string
	proxy      http.Handler
	sessionKey []byte
}

// sessionToken derives the cookie value from the admin ID. The key is random
// per process, so a token from an earlier run — or from another installation —
// never validates.
func (p *panelServer) sessionToken() string {
	mac := hmac.New(sha256.New, p.sessionKey)
	mac.Write([]byte(p.adminID))
	return hex.EncodeToString(mac.Sum(nil))
}

// NewAPIProxy forwards the panel's data calls to the Gamaj API using the bot's
// dedicated API key. The path arrives already reduced to /bot/... because the
// panel handler refuses every other prefix before reaching the proxy.
func NewAPIProxy(target *url.URL, apiKey string) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.URL.Path = "/api" + r.URL.Path
		r.URL.RawPath = ""
		// The panel session cookie is strictly for the browser and must never
		// travel upstream; the API key is the bot's own credential.
		r.Header.Del("Cookie")
		r.Header.Set("Authorization", "Bearer "+apiKey)
		r.Header.Set("Accept", "application/json")
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("botpanel: Gamaj API proxy: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":"Gamaj API request failed"}`))
	}
	return proxy
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
	case path == "api/admin":
		p.handleLogin(w, r)
	case strings.HasPrefix(path, proxyPrefix):
		if !p.authorized(r) {
			http.Error(w, `{"detail":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		r.URL.Path = "/" + strings.TrimPrefix(path, "api/")
		p.proxy.ServeHTTP(w, r)
	case strings.HasPrefix(path, "api/"):
		http.Error(w, `{"detail":"not found"}`, http.StatusNotFound)
	default:
		http.Redirect(w, r, "/panel", http.StatusMovedPermanently)
	}
}

// handleLogin validates the submitted admin ID server-side. The comparison
// happens here, not in the page, and a successful attempt is the only way to
// obtain a session cookie.
func (p *panelServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"detail":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	candidate := strings.TrimSpace(r.Header.Get(adminHeader))
	if candidate == "" || !constantTimeEquals(candidate, p.adminID) {
		http.Error(w, `{"detail":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	// HttpOnly keeps the token away from scripts, and SameSite=Strict stops a
	// cross-site page from riding along on the session.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    p.sessionToken(),
		Path:     "/panel",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// authorized checks the panel session cookie issued by handleLogin.
func (p *panelServer) authorized(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	return constantTimeEquals(cookie.Value, p.sessionToken())
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
