package web

import (
	"net/http"
)

// The data-admin API: list and edit a guild's bingo squares from the website.
// Every endpoint requires the caller to be a bingo admin of the guild - the same
// rule the Discord /bingo-data commands use, enforced again in the service layer.

// requireDataAdmin resolves the session, membership, and admin rights.
func (s *Server) requireDataAdmin(w http.ResponseWriter, r *http.Request, guildID string) (string, bool) {
	userID, ok := s.requireMember(w, r, guildID)
	if !ok {
		return "", false
	}
	admin, err := s.svc.IsAdmin(r.Context(), guildID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check permissions")
		return "", false
	}
	if !admin {
		writeError(w, http.StatusForbidden, "you are not a bingo admin of this server")
		return "", false
	}
	return userID, true
}

// handleDataOverview returns every pool with its squares (one flat list; all pools
// are equal and deletable now).
func (s *Server) handleDataOverview(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	if _, ok := s.requireDataAdmin(w, r, gid); !ok {
		return
	}
	pools, err := s.poolsWithEntries(r, gid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load data")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pools": pools})
}

func (s *Server) poolsWithEntries(r *http.Request, guildID string) ([]map[string]any, error) {
	pools, err := s.store.ListPools(r.Context(), guildID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(pools))
	for _, p := range pools {
		entries, err := s.store.ListEntries(r.Context(), guildID, p.ID, true)
		if err != nil {
			return nil, err
		}
		es := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			es = append(es, map[string]any{"id": e.ID, "text": e.Text})
		}
		out = append(out, map[string]any{"id": p.ID, "slug": p.Slug, "name": p.Name, "entries": es})
	}
	return out, nil
}

// poolsBrief returns each pool with its active-square count, for the new-game
// pool picker (which does not need the full square texts).
func (s *Server) poolsBrief(r *http.Request, guildID string) ([]map[string]any, error) {
	pools, err := s.store.ListPools(r.Context(), guildID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(pools))
	for _, p := range pools {
		entries, err := s.store.ListEntries(r.Context(), guildID, p.ID, true)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "count": len(entries)})
	}
	return out, nil
}

// handleDataEntryAdd adds a square to a pool.
func (s *Server) handleDataEntryAdd(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireDataAdmin(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		PoolID int64  `json:"poolId"`
		Text   string `json:"text"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	entry, err := s.svc.AddEntry(r.Context(), gid, userID, body.PoolID, body.Text)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": entry.ID, "text": entry.Text})
}

// handleDataEntryEdit updates a square's text.
func (s *Server) handleDataEntryEdit(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireDataAdmin(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		EntryID int64  `json:"entryId"`
		Text    string `json:"text"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.svc.EditEntry(r.Context(), gid, userID, body.EntryID, body.Text); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDataEntryRemove soft-deletes a square.
func (s *Server) handleDataEntryRemove(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireDataAdmin(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		EntryID int64 `json:"entryId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.svc.RemoveEntry(r.Context(), gid, userID, body.EntryID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDataPoolCreate creates a shared pool.
func (s *Server) handleDataPoolCreate(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireDataAdmin(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	pool, err := s.svc.CreatePool(r.Context(), gid, userID, body.Slug, body.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": pool.ID, "slug": pool.Slug, "name": pool.Name})
}

// handleDataPoolDelete removes a pool and its squares.
func (s *Server) handleDataPoolDelete(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireDataAdmin(w, r, gid)
	if !ok {
		return
	}
	var body struct {
		PoolID int64 `json:"poolId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.svc.DeletePool(r.Context(), gid, userID, body.PoolID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
