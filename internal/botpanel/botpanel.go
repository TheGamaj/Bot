// Package botpanel serves the Gamaj Bot Panel (is.0.0.1): the web management
// UI of Gamaj Bot. It is protected by a panel password and renders live sales
// data from the Gamaj API: plans, orders and their actions. Every data call is
// proxied to the Gamaj API with the bot's dedicated API key.
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
	"strconv"
	"strings"
	"time"
)

//go:embed index.html
var indexHTML string

// Version is the Gamaj Bot Panel release version.
const Version = "is.0.0.1"

const (
	// sessionCookie is the panel session cookie. Its value is an HMAC over the
	// panel credential's hash, so the value alone cannot be forged.
	sessionCookie = "gamaj_bot_panel"
	// adminHeader carries the candidate credential during login.
	adminHeader = "X-Panel-Admin-Id"
	// proxyPrefix is the only part of the Gamaj API this panel may reach. The
	// bot's API key is attached to every proxied call, so exposing anything
	// else would let a browser borrow the bot's panel-wide authority.
	proxyPrefix = "api/bot/"
	// sessionMaxAge bounds how long a login stays valid.
	sessionMaxAge = 12 * time.Hour
	// generatedTokenBytes is the length of a one-time token minted when no
	// password is configured.
	generatedTokenBytes = 32
)

// Handler serves the Bot Panel page on /panel and its JSON proxy on
// /panel/api/*. Login compares the submitted credential against the
// configured one, then issues a signed session cookie.
//
// password is the panel credential from the bot configuration. When it is
// empty a random one-time token is generated for this run and written to the
// log, so an operator who has not configured a password still has a way in
// while nobody who guesses a Telegram ID does. The plaintext is never stored:
// only its SHA-256 is, and it is compared in constant time.
func Handler(adminID, password string, proxy http.Handler) http.Handler {
	return newPanelServer(adminID, password, proxy).routes()
}

// newPanelServer builds the panel. It is separate from Handler so tests can
// reach the limiter and drive its clock without sleeping.
func newPanelServer(adminID, password string, proxy http.Handler) *panelServer {
	sessionKey := make([]byte, 32)
	if _, err := rand.Read(sessionKey); err != nil {
		// A predictable key would make every session forgeable, so refuse to
		// serve the panel at all rather than fall back to a weak secret.
		panic("botpanel: cannot seed the session key: " + err.Error())
	}
	credential := strings.TrimSpace(password)
	if credential == "" {
		generated, err := randomToken(generatedTokenBytes)
		if err != nil {
			panic("botpanel: cannot generate a panel token: " + err.Error())
		}
		credential = generated
		log.Printf("botpanel: no panel_password configured; generated a one-time token for this run: %s", credential)
		log.Printf("botpanel: set panel_password in the Gamaj Bot configuration to choose your own")
	}
	return &panelServer{
		adminID:    strings.TrimSpace(adminID),
		credential: credentialDigest(credential),
		proxy:      proxy,
		sessionKey: sessionKey,
		limiter:    defaultLimiter(),
	}
}

// routes mounts the panel page and its JSON proxy.
func (p *panelServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/panel", http.HandlerFunc(p.handlePage))
	mux.Handle("/panel/", http.HandlerFunc(p.handle))
	return mux
}

type panelServer struct {
	adminID    string
	credential [sha256.Size]byte
	proxy      http.Handler
	sessionKey []byte
	limiter    *limiter
}

// randomToken returns hex-encoded random bytes.
func randomToken(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// credentialDigest hashes the panel credential. Hashing rather than storing
// the plaintext means a memory dump of the process does not hand over the
// password itself.
func credentialDigest(credential string) [sha256.Size]byte {
	return sha256.Sum256([]byte(credential))
}

// sessionToken derives the cookie value from the credential hash. The key is
// random per process, so a token from an earlier run — or from another
// installation — never validates, and rotating the password invalidates every
// outstanding session at once.
func (p *panelServer) sessionToken() string {
	mac := hmac.New(sha256.New, p.sessionKey)
	mac.Write(p.credential[:])
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

// handleLogin validates the submitted credential server-side. The comparison
// happens here, not in the page, and a successful attempt is the only way to
// obtain a session cookie.
//
// A source address that fails repeatedly is locked out with a backoff that
// doubles each time, so the credential cannot be guessed by repetition. The
// admin Telegram ID used to be the credential; it is still accepted as a
// legacy fallback only when no password is configured, because it is not a
// secret.
func (p *panelServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"detail":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	source := sourceAddress(r)
	if wait, ok := p.limiter.allow(source); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		http.Error(w, `{"detail":"too many attempts"}`, http.StatusTooManyRequests)
		return
	}
	candidate := strings.TrimSpace(r.Header.Get(adminHeader))
	candidateDigest := credentialDigest(candidate)
	if candidate == "" || !constantTimeEqualsDigest(candidateDigest, p.credential) {
		p.limiter.fail(source)
		http.Error(w, `{"detail":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	p.limiter.succeed(source)
	// HttpOnly keeps the token away from scripts, SameSite=Strict stops a
	// cross-site page from riding along on the session, and MaxAge bounds how
	// long a stolen cookie is useful.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    p.sessionToken(),
		Path:     "/panel",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionMaxAge / time.Second),
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

// constantTimeEqualsDigest compares two fixed-size digests. Both operands are
// always the same length, so unlike constantTimeEquals it never short-circuits
// on length and leaks nothing through timing.
func constantTimeEqualsDigest(a, b [sha256.Size]byte) bool {
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
