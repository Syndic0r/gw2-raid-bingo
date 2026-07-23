package events

import (
	"testing"
	"time"
)

func TestSubscribeReceivesMatchingTopic(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(Topic("g1", 1))
	defer sub.Close()

	h.Publish(Event{Kind: CellToggled, GuildID: "g1", GameID: 1, CardID: 5})
	select {
	case e := <-sub.C:
		if e.Kind != CellToggled || e.CardID != 5 {
			t.Fatalf("unexpected event %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive event")
	}
}

func TestOtherTopicNotDelivered(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(Topic("g1", 1))
	defer sub.Close()
	h.Publish(Event{Kind: CellToggled, GuildID: "g1", GameID: 2})
	select {
	case e := <-sub.C:
		t.Fatalf("received event for wrong topic: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCloseStopsDelivery(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(Topic("g1", 1))
	sub.Close()
	// Publishing after close must not panic.
	h.Publish(Event{Kind: GameOpened, GuildID: "g1", GameID: 1})
	if _, ok := <-sub.C; ok {
		t.Fatal("channel should be closed and drained")
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(Topic("g1", 1))
	defer sub.Close()
	// Overfill the buffer; publishing must not block even though nobody reads.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.Publish(Event{Kind: CellToggled, GuildID: "g1", GameID: 1})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a slow subscriber")
	}
}

func TestSubscribeAllReceivesEveryTopic(t *testing.T) {
	h := NewHub()
	all := h.SubscribeAll()
	defer all.Close()
	h.Publish(Event{Kind: CellToggled, GuildID: "g1", GameID: 1})
	h.Publish(Event{Kind: GameOpened, GuildID: "g2", GameID: 5})
	for n := 0; n < 2; n++ {
		select {
		case <-all.C:
		case <-time.After(time.Second):
			t.Fatal("global subscriber missed an event")
		}
	}
}

func TestMultipleSubscribers(t *testing.T) {
	h := NewHub()
	a := h.Subscribe(Topic("g1", 1))
	b := h.Subscribe(Topic("g1", 1))
	defer a.Close()
	defer b.Close()
	h.Publish(Event{Kind: GameFinished, GuildID: "g1", GameID: 1})
	for _, sub := range []*Subscription{a, b} {
		select {
		case <-sub.C:
		case <-time.After(time.Second):
			t.Fatal("a subscriber missed the event")
		}
	}
}
