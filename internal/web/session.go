package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

const (
	sessionCookie  = "gw2bingo_session"
	stateCookie    = "gw2bingo_oauth_state"
	verifierCookie = "gw2bingo_oauth_verifier"
	sessionTTL     = 30 * 24 * time.Hour
)

// hashToken returns the hex SHA-256 of a token; only the hash is stored.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// setCookie writes an HttpOnly cookie. Secure is set for HTTPS deployments;
// SameSite=Lax lets the OAuth redirect back carry the session while still
// blocking cross-site POSTs.
func (s *Server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// startSession creates a server-side session and sets the cookie.
func (s *Server) startSession(ctx context.Context, w http.ResponseWriter, u DiscordUser) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL).Unix()
	if err := s.store.CreateSession(ctx, hashToken(token), u.ID, u.Username, u.Avatar, expires); err != nil {
		return err
	}
	s.setCookie(w, sessionCookie, token, sessionTTL)
	return nil
}

// currentUser returns the logged-in session, or false.
func (s *Server) currentUser(r *http.Request) (store.Session, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.Session{}, false
	}
	sess, err := s.store.GetSession(r.Context(), hashToken(c.Value))
	if err != nil {
		return store.Session{}, false
	}
	return sess, true
}

// endSession deletes the current session and clears the cookie.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = s.store.DeleteSession(r.Context(), hashToken(c.Value))
	}
	s.clearCookie(w, sessionCookie)
}
