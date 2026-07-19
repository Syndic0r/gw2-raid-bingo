package store

import (
	"context"
	"errors"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	future := now() + 3600

	if err := s.CreateSession(ctx, "hash-1", "user-1", "alice", "av", future); err != nil {
		t.Fatal(err)
	}
	sess, err := s.GetSession(ctx, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.UserID != "user-1" || sess.Username != "alice" {
		t.Fatalf("unexpected session %+v", sess)
	}

	if err := s.DeleteSession(ctx, "hash-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, "hash-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session: got %v, want ErrNotFound", err)
	}
}

func TestExpiredSessionNotReturnedAndPurged(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	past := now() - 10

	if err := s.CreateSession(ctx, "hash-old", "u", "bob", "", past); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, "hash-old"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session should be hidden: %v", err)
	}
	n, err := s.PurgeExpiredSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d, want 1", n)
	}
}
