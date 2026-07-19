package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// writeJSON encodes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleMe reports the login state and client config (bot invite availability).
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"loggedIn":     false,
		"loginEnabled": s.cfg.RequireWeb() == nil,
		"botUrl":       s.cfg.BotBaseURL, // the public "add the bot" landing site
		"version":      s.cfg.Version,
	}
	if sess, ok := s.currentUser(r); ok {
		resp["loggedIn"] = true
		resp["user"] = map[string]string{
			"id":       sess.UserID,
			"username": sess.Username,
			"avatar":   sess.Avatar,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGuilds lists the user's guilds that the bot is also in, each flagged with
// whether the user is a bingo admin there.
func (s *Server) handleGuilds(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	// The guild picker is the user's guilds intersected with the guilds the bot
	// is in. We enumerate the bot's guilds (a small set) and keep those where the
	// user is a member; this avoids persisting the user's OAuth token just to
	// re-fetch their guild list. Membership is resolved via the bot token.
	type guildOut struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Icon  string `json:"icon"`
		Admin bool   `json:"admin"`
	}
	out := []guildOut{}
	for _, id := range s.bot.GuildIDs() {
		if !s.svc.IsMember(r.Context(), id, sess.UserID) {
			continue
		}
		admin, _ := s.svc.IsAdmin(r.Context(), id, sess.UserID)
		out = append(out, guildOut{ID: id, Name: s.bot.GuildName(id), Icon: s.bot.GuildIcon(id), Admin: admin})
	}
	writeJSON(w, http.StatusOK, map[string]any{"guilds": out})
}

// requireMember resolves the session and verifies guild membership, returning
// the user id or writing an error.
func (s *Server) requireMember(w http.ResponseWriter, r *http.Request, guildID string) (string, bool) {
	sess, ok := s.currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return "", false
	}
	if !s.bot.InGuild(guildID) || !s.svc.IsMember(r.Context(), guildID, sess.UserID) {
		writeError(w, http.StatusForbidden, "not a member of this server")
		return "", false
	}
	return sess.UserID, true
}

// handleBoard returns the open game, the user's card, and stats for an instance.
func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	inst, err := bingo.ParseInstance(r.URL.Query().Get("instance"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance")
		return
	}
	admin, _ := s.svc.IsAdmin(r.Context(), gid, userID)

	resp := map[string]any{"instance": string(inst), "admin": admin}

	game, err := s.store.GetOpenGame(r.Context(), gid, inst)
	if errors.Is(err, store.ErrNotFound) {
		resp["game"] = nil
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load game")
		return
	}
	stats, err := s.svc.GameStatsForInstance(r.Context(), gid, inst)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	resp["game"] = gameJSON(game)
	resp["players"] = stats.PlayerCount
	resp["leaders"] = leadersJSON(stats.Leaders)

	if card, err := s.store.GetUserCard(r.Context(), game.ID, userID); err == nil {
		resp["card"] = cardJSON(card)
		resp["hasBingo"] = bingo.HasBingo(card.Marks())
	} else {
		resp["card"] = nil
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleHistory returns recent finished/aborted games.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	if _, ok := s.requireMember(w, r, gid); !ok {
		return
	}
	games, err := s.store.ListRecentGames(r.Context(), gid, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load history")
		return
	}
	out := make([]map[string]any, 0, len(games))
	for _, g := range games {
		out = append(out, gameJSON(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

// handleDeal deals (or returns) the user's card for an instance's open game.
func (s *Server) handleDeal(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		Instance string `json:"instance"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	inst, err := bingo.ParseInstance(body.Instance)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance")
		return
	}
	card, _, err := s.svc.DealCard(r.Context(), gid, userID, inst)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": cardJSON(card)})
}

// handleToggle flips a cell on the user's own card.
func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		CardID int64 `json:"cardId"`
		Index  int   `json:"index"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	card, hasBingo, err := s.svc.ToggleCell(r.Context(), gid, userID, body.CardID, body.Index)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": cardJSON(card), "hasBingo": hasBingo})
}

// handleCall finalizes a win.
func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		CardID int64 `json:"cardId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	res, err := s.svc.CallBingo(r.Context(), gid, userID, body.CardID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"won": true, "instance": string(res.Game.Instance)})
}

// handleNewGame opens a game (admin only).
func (s *Server) handleNewGame(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		Instance string `json:"instance"`
		Replace  bool   `json:"replace"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	inst, err := bingo.ParseInstance(body.Instance)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance")
		return
	}
	pools := s.allSharedPoolIDs(r.Context(), gid)
	game, err := s.svc.NewGame(r.Context(), gid, userID, inst, pools, body.Replace)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": gameJSON(game)})
}

// handleAbortGame aborts the open game for an instance (admin only).
func (s *Server) handleAbortGame(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		Instance string `json:"instance"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	inst, err := bingo.ParseInstance(body.Instance)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance")
		return
	}
	if _, err := s.svc.AbortGame(r.Context(), gid, userID, inst); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"aborted": true})
}

func (s *Server) allSharedPoolIDs(ctx context.Context, guildID string) []int64 {
	pools, err := s.store.ListPools(ctx, guildID, store.KindShared)
	if err != nil {
		return nil
	}
	ids := make([]int64, 0, len(pools))
	for _, p := range pools {
		ids = append(ids, p.ID)
	}
	return ids
}

// writeServiceError maps service/store errors to HTTP responses.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "you do not have permission to do that")
	case errors.Is(err, service.ErrNoAnnounceChannel):
		writeError(w, http.StatusBadRequest, "set an announcement channel in Discord with /setup first")
	case errors.Is(err, bingo.ErrNotEnoughEntries):
		writeError(w, http.StatusBadRequest, "not enough squares for this instance yet (24 needed)")
	case errors.Is(err, store.ErrGameNotOpen):
		writeError(w, http.StatusConflict, "that game is not open")
	case errors.Is(err, store.ErrGameOpen):
		writeError(w, http.StatusConflict, "a game is already open for this instance")
	case errors.Is(err, store.ErrNoBingo):
		writeError(w, http.StatusBadRequest, "that card has no completed line yet")
	case errors.Is(err, store.ErrCellFree):
		writeError(w, http.StatusBadRequest, "the free centre cannot be toggled")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "something went wrong")
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
