// Package events is a small in-process publish/subscribe hub. State changes to a
// game (a cell toggled, a card dealt, a game opened or finished) are published
// once and fanned out to every interested subscriber: the web server's SSE
// connections and the Discord live-message updater. This is what keeps a toggle
// in Discord and the website in sync without any cross-process coordination.
package events

import "sync"

// Kind enumerates the kinds of state change.
type Kind string

const (
	GameOpened   Kind = "game_opened"
	GameFinished Kind = "game_finished"
	GameAborted  Kind = "game_aborted"
	CardDealt    Kind = "card_dealt"
	CellToggled  Kind = "cell_toggled"
)

// Event describes one state change. Fields not relevant to a Kind are zero.
type Event struct {
	Kind     Kind
	GuildID  string
	Instance string
	GameID   int64
	CardID   int64
	UserID   string
}

// Topic groups events a subscriber cares about: everything for one instance in
// one guild.
func Topic(guildID, instance string) string { return guildID + ":" + instance }

// Hub fans events out to subscribers. It is safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	subs    map[string]map[int]chan Event
	globals map[int]chan Event
	next    int
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[int]chan Event), globals: make(map[int]chan Event)}
}

// Subscription is a live subscription; read from C, and call Close when done.
// A global subscription (SubscribeAll) has an empty topic.
type Subscription struct {
	C      <-chan Event
	hub    *Hub
	topic  string
	id     int
	global bool
}

// Subscribe registers interest in a topic. The returned channel is buffered;
// Close must be called to release it.
func (h *Hub) Subscribe(topic string) *Subscription {
	ch := make(chan Event, 16)
	h.mu.Lock()
	id := h.next
	h.next++
	if h.subs[topic] == nil {
		h.subs[topic] = make(map[int]chan Event)
	}
	h.subs[topic][id] = ch
	h.mu.Unlock()
	return &Subscription{C: ch, hub: h, topic: topic, id: id}
}

// SubscribeAll registers interest in every event regardless of topic. The bot's
// live-message updater uses this to maintain status messages across all guilds.
func (h *Hub) SubscribeAll() *Subscription {
	ch := make(chan Event, 64)
	h.mu.Lock()
	id := h.next
	h.next++
	h.globals[id] = ch
	h.mu.Unlock()
	return &Subscription{C: ch, hub: h, id: id, global: true}
}

// Close removes the subscription and closes its channel.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if s.global {
		if ch, ok := s.hub.globals[s.id]; ok {
			delete(s.hub.globals, s.id)
			close(ch)
		}
		return
	}
	if subs, ok := s.hub.subs[s.topic]; ok {
		if ch, ok := subs[s.id]; ok {
			delete(subs, s.id)
			close(ch)
		}
		if len(subs) == 0 {
			delete(s.hub.subs, s.topic)
		}
	}
}

// Publish delivers e to every subscriber of its topic. Delivery is non-blocking:
// if a subscriber's buffer is full the event is dropped for that subscriber
// rather than stalling the game (SSE clients reconcile from a full fetch on
// reconnect, and the Discord updater re-reads current state).
func (h *Hub) Publish(e Event) {
	topic := Topic(e.GuildID, e.Instance)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs[topic] {
		select {
		case ch <- e:
		default:
		}
	}
	for _, ch := range h.globals {
		select {
		case ch <- e:
		default:
		}
	}
}
