package botpanel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// panelPassword is the credential the tests sign in with. It is deliberately
// nothing like the admin ID, because the whole point of the change is that the
// two are no longer the same thing.
const panelPassword = "correct-horse-battery-staple"

// adminID is the bot's configured admin Telegram ID. It is public information,
// which is exactly why it must not be a credential.
const adminID = "4242"

// stubProxy stands in for the upstream proxy so the panel's own decisions can
// be observed. Header hygiene towards the real API is the proxy's job and is
// covered by TestAPIProxyTargetContract.
type stubProxy struct {
	calls  []string
	status int
	body   string
}

func (s *stubProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.calls = append(s.calls, r.URL.Path)
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	body := s.body
	if body == "" {
		body = "[]"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// tryLogin performs a login from a given source address and returns the recorder.
func tryLogin(handler http.Handler, credential, source string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/panel/api/admin", nil)
	request.RemoteAddr = source + ":50000"
	if credential != "" {
		request.Header.Set(adminHeader, credential)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// login performs the panel login and returns the issued session cookie.
func login(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := tryLogin(handler, panelPassword, "10.0.0.1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookie {
			return cookie
		}
	}
	t.Fatal("login did not issue a session cookie")
	return nil
}

func TestPanelPageAndUnauthenticatedAPI(t *testing.T) {
	proxy := &stubProxy{}
	handler := Handler(adminID, panelPassword, proxy)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panel", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("/panel status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Coded by AsliCode") {
		t.Error("/panel did not render the Gamaj author footer")
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panel/api/bot/plans", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /panel/api/bot/plans status = %d, want 401", recorder.Code)
	}
	if len(proxy.calls) != 0 {
		t.Errorf("the proxy was called without a session: %v", proxy.calls)
	}
}

// TestLoginRejectsTheAdminID is the regression this whole change exists for.
//
// The admin Telegram ID used to be the credential. Anyone who knew it - and it
// is not a secret: it appears in bot logs, in webhook payloads and in any chat
// the bot is in - could sign in and reach the whole bot sales API through the
// panel's proxy. Knowing the ID must no longer be enough.
func TestLoginRejectsTheAdminID(t *testing.T) {
	proxy := &stubProxy{}
	handler := Handler(adminID, panelPassword, proxy)

	for _, credential := range []string{adminID, "", " " + adminID, "42420", "0"} {
		recorder := tryLogin(handler, credential, "10.0.0.1")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("login with %q status = %d, want 401", credential, recorder.Code)
		}
		if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
			t.Errorf("a rejected login issued cookies: %v", cookies)
		}
	}
	if len(proxy.calls) != 0 {
		t.Errorf("a rejected login reached the proxy: %v", proxy.calls)
	}
}

// TestGeneratedTokenRejectsTheAdminID covers the unconfigured case. With no
// panel_password the panel mints a random token, so the admin ID is no better
// than any other guess.
func TestGeneratedTokenRejectsTheAdminID(t *testing.T) {
	handler := Handler(adminID, "", &stubProxy{})

	if recorder := tryLogin(handler, adminID, "10.0.0.1"); recorder.Code != http.StatusUnauthorized {
		t.Errorf("login with the admin ID and no configured password: status = %d, want 401", recorder.Code)
	}
	// The generated token is 32 random bytes, so it is not guessable and is
	// definitely not derived from the admin ID.
	token, err := randomToken(generatedTokenBytes)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if token == adminID || len(token) != generatedTokenBytes*2 {
		t.Errorf("generated token = %q, want %d hex characters", token, generatedTokenBytes*2)
	}
}

// TestLoginIsRateLimited proves the credential cannot be guessed by repetition.
//
// A four-digit admin ID has ten thousand possibilities; without a limiter that
// is a handful of seconds of requests. The backoff has to hold.
func TestLoginIsRateLimited(t *testing.T) {
	panel := newPanelServer(adminID, panelPassword, &stubProxy{})
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	panel.limiter.now = func() time.Time { return now }
	handler := panel.routes()

	threshold := panel.limiter.threshold
	// The threshold is allowed through and answered 401, not 429: only a run
	// of failures triggers a lockout.
	for i := 0; i < threshold; i++ {
		if recorder := tryLogin(handler, "wrong", "10.0.0.9"); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, recorder.Code)
		}
	}
	if recorder := tryLogin(handler, "wrong", "10.0.0.9"); recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt after %d failures status = %d, want 429", threshold, recorder.Code)
	}
	// Retry-After has to be present and honest, otherwise a client cannot know
	// when to come back.
	if recorder := tryLogin(handler, "wrong", "10.0.0.9"); recorder.Header().Get("Retry-After") == "" {
		t.Error("a locked-out login did not set Retry-After")
	}

	// The correct password is refused too while the lockout stands. Otherwise
	// the lockout would be a denial of service against the real operator.
	if recorder := tryLogin(handler, panelPassword, "10.0.0.9"); recorder.Code != http.StatusTooManyRequests {
		t.Errorf("the correct password during a lockout: status = %d, want 429", recorder.Code)
	}

	// A different source is unaffected: the limit is per address.
	if recorder := tryLogin(handler, panelPassword, "10.0.0.2"); recorder.Code != http.StatusOK {
		t.Errorf("a different source was locked out too: status = %d, want 200", recorder.Code)
	}

	// Once the backoff expires the address is let back in.
	now = now.Add(panel.limiter.base + time.Second)
	if recorder := tryLogin(handler, panelPassword, "10.0.0.9"); recorder.Code != http.StatusOK {
		t.Errorf("login after the lockout expired: status = %d, want 200", recorder.Code)
	}
}

// TestLockoutBackoffGrows checks the second lockout is longer than the first,
// so a sustained attack gets slower rather than cycling every base interval.
func TestLockoutBackoffGrows(t *testing.T) {
	panel := newPanelServer(adminID, panelPassword, &stubProxy{})
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	panel.limiter.now = func() time.Time { return now }
	handler := panel.routes()

	measure := func() time.Duration {
		for i := 0; i < panel.limiter.threshold; i++ {
			tryLogin(handler, "wrong", "10.0.0.8")
		}
		// allow reports (wait, ok); ok is false while the source is locked out.
		wait, ok := panel.limiter.allow("10.0.0.8")
		if ok {
			t.Fatal("expected the source to be locked out")
		}
		return wait
	}
	first := measure()
	now = now.Add(first + time.Second)
	second := measure()
	if second <= first {
		t.Errorf("second lockout = %v, want longer than the first (%v)", second, first)
	}
	if second > panel.limiter.cap {
		t.Errorf("second lockout = %v, over the cap %v", second, panel.limiter.cap)
	}
}

// TestSuccessfulLoginClearsTheRecord stops a mistyped password from locking
// the operator out for ever: one success resets the counter.
func TestSuccessfulLoginClearsTheRecord(t *testing.T) {
	panel := newPanelServer(adminID, panelPassword, &stubProxy{})
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	panel.limiter.now = func() time.Time { return now }
	handler := panel.routes()

	for i := 0; i < panel.limiter.threshold-1; i++ {
		tryLogin(handler, "wrong", "10.0.0.7")
	}
	if recorder := tryLogin(handler, panelPassword, "10.0.0.7"); recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d", recorder.Code)
	}
	for i := 0; i < panel.limiter.threshold-1; i++ {
		if recorder := tryLogin(handler, "wrong", "10.0.0.7"); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401: the success did not reset the counter", i+1, recorder.Code)
		}
	}
}

func TestSessionCookieIsNotForgeableAndIsHttpOnly(t *testing.T) {
	proxy := &stubProxy{body: `[{"name":"plan"}]`}
	handler := Handler(adminID, panelPassword, proxy)

	cookie := login(t, handler)
	if !cookie.HttpOnly {
		t.Error("the session cookie is readable by scripts")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("the session cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	// A stolen cookie has to stop working on its own eventually.
	if cookie.MaxAge <= 0 {
		t.Error("the session cookie has no MaxAge, so it never expires")
	}
	if cookie.Value == panelPassword || cookie.Value == adminID {
		t.Fatal("the session cookie is the credential itself, so it can be replayed as one")
	}

	// The cookie grants access, and the proxy runs with the bot's own key while
	// the browser cookie stays behind.
	request := httptest.NewRequest(http.MethodGet, "/panel/api/bot/plans", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authorized /panel/api/bot/plans status = %d, want 200", recorder.Code)
	}
	if len(proxy.calls) != 1 || proxy.calls[0] != "/bot/plans" {
		t.Fatalf("proxy calls = %v, want [/bot/plans]", proxy.calls)
	}

	// Cookies an attacker could build from public information must all fail.
	for _, forged := range []string{panelPassword, adminID, "", "x"} {
		attempt := httptest.NewRequest(http.MethodGet, "/panel/api/bot/plans", nil)
		attempt.AddCookie(&http.Cookie{Name: sessionCookie, Value: forged})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, attempt)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("forged cookie %q status = %d, want 401", forged, recorder.Code)
		}
	}
	if len(proxy.calls) != 1 {
		t.Errorf("a forged cookie reached the proxy: %v", proxy.calls)
	}
}

// TestLoginEndpointIsPostOnly keeps a browser prefetch from minting a session.
func TestLoginEndpointIsPostOnly(t *testing.T) {
	handler := Handler(adminID, panelPassword, &stubProxy{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panel/api/admin", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /panel/api/admin status = %d, want 405", recorder.Code)
	}
}

func TestProxyOnlyReachesTheBotSurface(t *testing.T) {
	proxy := &stubProxy{}
	handler := Handler(adminID, panelPassword, proxy)
	cookie := login(t, handler)

	// The proxy attaches the bot's API key, which can reach the whole panel.
	// Any path the panel does not own must be refused before the proxy sees it.
	// /panel/api/admin is deliberately absent: it is the login endpoint, so a
	// GET on it answers 405 from the login handler instead.
	for _, path := range []string{
		"/panel/api/bot",
		"/panel/api/users",
		"/panel/api/auth/login",
		"/panel/api/settings",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, recorder.Code)
		}
	}
	if len(proxy.calls) != 0 {
		t.Errorf("a non-bot path reached the proxy: %v", proxy.calls)
	}
}

// TestAPIProxyTargetContract pins the real upstream request the panel builds:
// it must keep the query string, add the /api prefix, carry the bot's API key
// and never forward the browser cookie.
func TestAPIProxyTargetContract(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(adminID, panelPassword, NewAPIProxy(target, "bot-api-key"))
	cookie := login(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/panel/api/bot/orders?limit=50", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotPath != "/api/bot/orders" {
		t.Errorf("upstream path = %q, want /api/bot/orders", gotPath)
	}
	if gotQuery != "limit=50" {
		t.Errorf("upstream query = %q, want limit=50", gotQuery)
	}
	if gotAuth != "Bearer bot-api-key" {
		t.Errorf("upstream Authorization = %q, want the bot API key", gotAuth)
	}
	if gotCookie != "" {
		t.Errorf("the browser cookie was forwarded upstream: %q", gotCookie)
	}
}
