package botpanel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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

// login performs the panel login and returns the issued session cookie.
func login(t *testing.T, handler http.Handler, adminID string) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/panel/api/admin", nil)
	request.Header.Set(adminHeader, adminID)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
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
	handler := Handler("4242", proxy)

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

func TestLoginRejectsWrongAdminID(t *testing.T) {
	proxy := &stubProxy{}
	handler := Handler("4242", proxy)

	request := httptest.NewRequest(http.MethodPost, "/panel/api/admin", nil)
	request.Header.Set(adminHeader, "1111")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("login with a foreign admin ID status = %d, want 401", recorder.Code)
	}
	if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("a rejected login issued cookies: %v", cookies)
	}

	// The login endpoint is POST only, so a browser prefetch cannot mint one.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panel/api/admin", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /panel/api/admin status = %d, want 405", recorder.Code)
	}
}

func TestSessionCookieIsNotForgeableAndIsHttpOnly(t *testing.T) {
	proxy := &stubProxy{body: `[{"name":"plan"}]`}
	handler := Handler("4242", proxy)

	cookie := login(t, handler, "4242")
	if !cookie.HttpOnly {
		t.Error("the session cookie is readable by scripts")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("the session cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Value == "4242" {
		t.Fatal("the session cookie is the raw admin ID, so anyone who knows it can forge a session")
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

	// The forged cookie an attacker could build from the numeric admin ID — the
	// exact value the panel used to trust — must be rejected.
	forged := httptest.NewRequest(http.MethodGet, "/panel/api/bot/plans", nil)
	forged.AddCookie(&http.Cookie{Name: sessionCookie, Value: "4242"})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, forged)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("forged cookie status = %d, want 401", recorder.Code)
	}
	if len(proxy.calls) != 1 {
		t.Errorf("a forged cookie reached the proxy: %v", proxy.calls)
	}
}

func TestProxyOnlyReachesTheBotSurface(t *testing.T) {
	proxy := &stubProxy{}
	handler := Handler("4242", proxy)
	cookie := login(t, handler, "4242")

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
	handler := Handler("4242", NewAPIProxy(target, "bot-api-key"))
	cookie := login(t, handler, "4242")

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
