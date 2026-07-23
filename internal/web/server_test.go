package web

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/authz"
	"github.com/Syndic0r/gw2-raid-bingo/internal/config"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

type fakeResolver struct{}

func (fakeResolver) Resolve(context.Context, string, string) (authz.Member, error) {
	return authz.Member{}, nil
}

type fakeBot struct{ ids []string }

func (f fakeBot) InGuild(id string) bool {
	for _, x := range f.ids {
		if x == id {
			return true
		}
	}
	return false
}
func (f fakeBot) GuildIDs() []string         { return f.ids }
func (f fakeBot) GuildName(id string) string { return "Guild " + id }
func (f fakeBot) GuildIcon(string) string    { return "" }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(context.Background(), "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := service.New(st, events.NewHub(), fakeResolver{})
	cfg := config.Config{HTTPAddr: "127.0.0.1:0"}
	return NewServer(cfg, svc, events.NewHub(), fakeBot{ids: []string{"g1"}}, log.New(nil, "", 0))
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz = %d %q", rec.Code, rec.Body.String())
	}
}

func TestLandingServed(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "GW2 Raid Bingo") {
		t.Error("landing page missing title")
	}
}

func TestMeLoggedOut(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"loggedIn":false`) {
		t.Errorf("unexpected body %q", rec.Body.String())
	}
}

func TestGuildScopedRequiresLogin(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/guild/g1/games", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestGuildsRequiresLogin(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/guilds", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDataEndpointRequiresAdmin(t *testing.T) {
	s := newTestServer(t)
	// A logged-in member who is NOT an admin (fakeResolver returns a plain member).
	token := "tok-nonadmin"
	if err := s.store.CreateSession(context.Background(), hashToken(token), "u1", "user", "", 9_999_999_999); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/guild/g1/data", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin data access = %d, want 403", rec.Code)
	}
}

func TestBasePathMount(t *testing.T) {
	s := newTestServer(t)
	s.cfg.BasePath = "/play"

	// Under the prefix the SPA shell is served and gets a <base> tag pointing at it.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/play/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/play/ status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<base href="/play/">`) {
		t.Errorf("missing injected base tag: %q", rec.Body.String())
	}

	// The bare prefix redirects to the trailing-slash form so relative URLs resolve.
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/play", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/play/" {
		t.Fatalf("/play redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// API routes are reachable under the prefix.
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/play/api/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/play/api/me status %d", rec.Code)
	}

	// Embedded static assets are reachable under the prefix (StripPrefix -> fileserver).
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/play/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/play/assets/app.js status %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("asset missing no-cache header: %q", rec.Header().Get("Cache-Control"))
	}

	// The un-prefixed path is not served by the app (nginx owns / for the landing).
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code == http.StatusOK {
		t.Errorf("/api/me should not be served under the prefix mount, got %d", rec.Code)
	}
}

func TestCSRFOriginCheck(t *testing.T) {
	s := newTestServer(t)
	s.cfg.BaseURL = "https://example.org"

	// Cross-site Origin on a POST is rejected before any handler runs.
	req := httptest.NewRequest(http.MethodPost, "/api/guild/g1/toggle", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", rec.Code)
	}

	// Same-origin POST passes the CSRF gate (then fails auth, which is fine).
	req = httptest.NewRequest(http.MethodPost, "/api/guild/g1/toggle", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://example.org")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("same-origin POST = %d, want 401 (past the CSRF gate)", rec.Code)
	}

	// GETs are never origin-blocked.
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET with foreign origin = %d, want 200", rec.Code)
	}
}

func TestSSELimiter(t *testing.T) {
	s := newTestServer(t)
	for i := 0; i < maxSSEPerUser; i++ {
		if !s.acquireSSE("u1") {
			t.Fatalf("acquire %d should succeed", i+1)
		}
	}
	if s.acquireSSE("u1") {
		t.Fatal("acquire beyond the cap should fail")
	}
	if !s.acquireSSE("u2") {
		t.Fatal("another user must not be affected")
	}
	s.releaseSSE("u1")
	if !s.acquireSSE("u1") {
		t.Fatal("acquire after release should succeed")
	}
}
