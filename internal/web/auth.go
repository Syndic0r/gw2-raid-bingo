package web

import (
	"net/http"
	"net/url"
	"time"
)

// botInvitePermissions is the minimal permission integer the bot needs: view
// channels, send messages, embed links, attach files, read message history.
const botInvitePermissions = "117760"

// handleLogin starts the OAuth flow: it sets short-lived state and PKCE-verifier
// cookies and redirects to Discord.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.RequireWeb(); err != nil {
		http.Error(w, "login is not configured", http.StatusServiceUnavailable)
		return
	}
	state, err1 := randomToken(24)
	verifier, err2 := randomToken(48)
	if err1 != nil || err2 != nil {
		http.Error(w, "could not start login", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, stateCookie, state, 10*time.Minute)
	s.setCookie(w, verifierCookie, verifier, 10*time.Minute)
	http.Redirect(w, r, s.oauth.authCodeURL(state, verifier), http.StatusFound)
}

// handleCallback completes the OAuth flow and starts a session.
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")

	stateCk, err := r.Cookie(stateCookie)
	verifierCk, err2 := r.Cookie(verifierCookie)
	// The single-use flow cookies are cleared regardless of outcome.
	s.clearCookie(w, stateCookie)
	s.clearCookie(w, verifierCookie)
	if err != nil || err2 != nil || code == "" || state == "" || state != stateCk.Value {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}

	token, err := s.oauth.exchange(r.Context(), code, verifierCk.Value)
	if err != nil {
		s.log.Printf("web: oauth exchange: %v", err)
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	user, err := s.oauth.me(r.Context(), token)
	if err != nil {
		s.log.Printf("web: fetch user: %v", err)
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	if err := s.startSession(r.Context(), w, user); err != nil {
		s.log.Printf("web: start session: %v", err)
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleLogout ends the session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.endSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// handleInvite redirects to the bot's install URL.
func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AppID == "" {
		http.Error(w, "invite is not configured", http.StatusServiceUnavailable)
		return
	}
	q := url.Values{
		"client_id":   {s.cfg.AppID},
		"scope":       {"bot applications.commands"},
		"permissions": {botInvitePermissions},
	}
	http.Redirect(w, r, "https://discord.com/oauth2/authorize?"+q.Encode(), http.StatusFound)
}
