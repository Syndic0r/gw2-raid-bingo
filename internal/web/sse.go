package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
)

// handleEvents streams server-sent events for one instance's game. The client
// reconnects automatically (EventSource) and refetches the board on each event,
// so the payload stays tiny and a missed event self-heals on the next fetch or
// reconnect.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	gid := r.PathValue("gid")
	userID, ok := s.requireMember(w, r, gid)
	if !ok {
		return
	}
	if !s.acquireSSE(userID) {
		writeError(w, http.StatusTooManyRequests, "too many live connections - close another tab")
		return
	}
	defer s.releaseSSE(userID)
	inst, err := bingo.ParseInstance(r.URL.Query().Get("instance"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering for SSE

	sub := s.hub.Subscribe(events.Topic(gid, string(inst)))
	defer sub.Close()

	// Initial comment so the client knows the stream is open.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, ok := <-sub.C:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: {\"kind\":%q,\"gameId\":%d,\"cardId\":%d}\n\n", e.Kind, e.Kind, e.GameID, e.CardID)
			flusher.Flush()
		}
	}
}
