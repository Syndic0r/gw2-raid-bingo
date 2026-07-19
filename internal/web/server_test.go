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
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/guild/g1/board?instance=w1", nil))
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
