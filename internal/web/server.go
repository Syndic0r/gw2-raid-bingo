// Package web is the website: a public landing page with the bot invite, Discord
// OAuth login, a guild picker, and playable bingo cards synced live over SSE. It
// shares the store, service, and event hub with the bot, so a toggle on the site
// and a toggle in Discord update each other.
package web

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Syndic0r/gw2-raid-bingo/internal/config"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

//go:embed static/*
var staticFS embed.FS

// BotPresence exposes the guilds the bot is in, used to build the guild picker
// (the user's guilds intersected with these) and to authorize guild-scoped
// requests. Implemented by the Discord bot over its gateway state.
type BotPresence interface {
	InGuild(guildID string) bool
	GuildIDs() []string
	GuildName(guildID string) string
	GuildIcon(guildID string) string
}

// Server is the HTTP server.
type Server struct {
	cfg           config.Config
	svc           *service.Service
	store         *store.Store
	hub           *events.Hub
	oauth         *oauthClient
	bot           BotPresence
	log           *log.Logger
	mux           *http.ServeMux
	secureCookies bool
}

// NewServer builds the web server.
func NewServer(cfg config.Config, svc *service.Service, hub *events.Hub, bot BotPresence, logger *log.Logger) *Server {
	s := &Server{
		cfg:           cfg,
		svc:           svc,
		store:         svc.Store(),
		hub:           hub,
		oauth:         newOAuthClient(cfg.AppID, cfg.ClientSecret, cfg.RedirectURI()),
		bot:           bot,
		log:           logger,
		mux:           http.NewServeMux(),
		secureCookies: strings.HasPrefix(cfg.BaseURL, "https://"),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Static assets and the SPA shell. Assets are served with no-cache so a
	// deploy's CSS/JS/image changes show up on the next load instead of being
	// pinned by the browser cache (the payload is tiny).
	sub, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(sub))
	noCache := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			next.ServeHTTP(w, r)
		})
	}
	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.Handle("GET /assets/", noCache(http.StripPrefix("/assets/", fileServer)))

	// Auth.
	s.mux.HandleFunc("GET /auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /auth/callback", s.handleCallback)
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /invite", s.handleInvite)

	// API.
	s.mux.HandleFunc("GET /api/me", s.handleMe)
	s.mux.HandleFunc("GET /api/guilds", s.handleGuilds)
	s.mux.HandleFunc("GET /api/guild/{gid}/board", s.handleBoard)
	s.mux.HandleFunc("GET /api/guild/{gid}/history", s.handleHistory)
	s.mux.HandleFunc("POST /api/guild/{gid}/card", s.handleDeal)
	s.mux.HandleFunc("POST /api/guild/{gid}/toggle", s.handleToggle)
	s.mux.HandleFunc("POST /api/guild/{gid}/call", s.handleCall)
	s.mux.HandleFunc("POST /api/guild/{gid}/game/new", s.handleNewGame)
	s.mux.HandleFunc("POST /api/guild/{gid}/game/abort", s.handleAbortGame)
	s.mux.HandleFunc("GET /api/guild/{gid}/events", s.handleEvents)

	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
}

// Handler returns the http.Handler with security headers applied.
func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.mux)
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts down.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go s.purgeSessionsLoop(ctx)

	errc := make(chan error, 1)
	go func() {
		s.log.Printf("web: listening on %s", s.cfg.HTTPAddr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errc:
		return err
	}
}

// securityHeaders applies a strict CSP suited to a self-contained same-origin
// app (no external resources), plus the usual hardening headers.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: https://cdn.discordapp.com; "+
				"connect-src 'self'; form-action 'self' https://discord.com; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) purgeSessionsLoop(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.store.PurgeExpiredSessions(ctx); err != nil {
				s.log.Printf("web: purge sessions: %v", err)
			}
		}
	}
}

// handleIndex serves the game app shell; its logged-out view is a login screen.
// The public "add the bot" landing is a separate static site (repo web/), served
// by nginx under its own low-priv user - it shares nothing with this process.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
