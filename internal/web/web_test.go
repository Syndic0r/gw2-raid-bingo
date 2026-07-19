package web

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPKCEChallengeMatchesSpec(t *testing.T) {
	// RFC 7636 test vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := pkceChallenge(verifier); got != want {
		t.Fatalf("pkceChallenge = %q, want %q", got, want)
	}
}

func TestRandomTokenUnique(t *testing.T) {
	a, _ := randomToken(32)
	b, _ := randomToken(32)
	if a == b {
		t.Fatal("tokens should be unique")
	}
	if _, err := base64.RawURLEncoding.DecodeString(a); err != nil {
		t.Fatalf("token is not URL-safe base64: %v", err)
	}
}

func TestHashTokenStable(t *testing.T) {
	if hashToken("abc") != hashToken("abc") {
		t.Fatal("hash should be deterministic")
	}
	if hashToken("abc") == hashToken("abd") {
		t.Fatal("different tokens should hash differently")
	}
	if strings.Contains(hashToken("secret-token"), "secret-token") {
		t.Fatal("hash must not contain the raw token")
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := &Server{}
	h := s.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP missing default-src none: %q", csp)
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options DENY")
	}
}

func TestAuthCodeURL(t *testing.T) {
	c := newOAuthClient("appid", "secret", "https://example.org/auth/callback")
	u := c.authCodeURL("state123", "verifier456")
	for _, want := range []string{
		"client_id=appid",
		"response_type=code",
		"scope=identify+guilds",
		"state=state123",
		"code_challenge_method=S256",
		"redirect_uri=https%3A%2F%2Fexample.org%2Fauth%2Fcallback",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("auth URL missing %q in %q", want, u)
		}
	}
	if strings.Contains(u, "verifier456") {
		t.Error("raw PKCE verifier must not appear in the auth URL")
	}
}
