// Package service is the shared application layer over the store and the event
// hub. Every game action the Discord bot and the web server perform goes through
// here, so authorization (the single bingo-admin rule) and event publishing are
// enforced in exactly one place and cannot be forgotten by a caller.
package service

import (
	"context"
	"errors"
	"math/rand"

	"github.com/Syndic0r/gw2-raid-bingo/internal/authz"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// ErrForbidden is returned when the actor lacks permission for an action.
var ErrForbidden = errors.New("forbidden")

// ErrNoAnnounceChannel is returned when a game would be opened or scheduled
// before an announcement channel is configured, since that is where the win
// celebration is posted.
var ErrNoAnnounceChannel = errors.New("no announcement channel configured")

// RoleResolver reports a member's Discord standing in a guild (owner flag, their
// roles, and which of those roles carry the Administrator permission). The
// Discord layer implements this over the REST API; tests use a fake.
type RoleResolver interface {
	Resolve(ctx context.Context, guildID, userID string) (authz.Member, error)
}

// Service ties the store, event hub, and role resolution together.
type Service struct {
	store    *store.Store
	hub      *events.Hub
	resolver RoleResolver
	rng      func() *rand.Rand
}

// New builds a Service. rng may be nil, in which case the store's per-(game,
// user) deterministic seed is used when dealing cards.
func New(st *store.Store, hub *events.Hub, resolver RoleResolver) *Service {
	return &Service{store: st, hub: hub, resolver: resolver}
}

// Store exposes the underlying store for read-only queries the handlers need
// (listing pools, history, and so on) without duplicating pass-throughs.
func (s *Service) Store() *store.Store { return s.store }

// IsAdmin reports whether the user is a bingo admin in the guild.
func (s *Service) IsAdmin(ctx context.Context, guildID, userID string) (bool, error) {
	member, err := s.resolver.Resolve(ctx, guildID, userID)
	if err != nil {
		return false, err
	}
	roles, err := s.store.GetAdminRoles(ctx, guildID)
	if err != nil {
		return false, err
	}
	return authz.IsBingoAdmin(member, authz.GuildConfig{AdminRoleIDs: roles}), nil
}

// IsMember reports whether the user is a member of the guild. It is used by the
// web layer to authorize guild-scoped requests. Resolution uses the bot token,
// so a non-member (or a guild the bot is not in) yields false.
func (s *Service) IsMember(ctx context.Context, guildID, userID string) bool {
	_, err := s.resolver.Resolve(ctx, guildID, userID)
	return err == nil
}

// CanConfigure reports whether the user may run /setup (Discord Administrator
// only, not the configured bingo-admin roles).
func (s *Service) CanConfigure(ctx context.Context, guildID, userID string) (bool, error) {
	member, err := s.resolver.Resolve(ctx, guildID, userID)
	if err != nil {
		return false, err
	}
	return authz.RequireDiscordAdministrator(member), nil
}

func (s *Service) requireAdmin(ctx context.Context, guildID, userID string) error {
	ok, err := s.IsAdmin(ctx, guildID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

// requireAnnounceChannel ensures a win destination is configured before a game
// can be opened or scheduled. Games can still be started from any channel; this
// only guarantees the celebration has somewhere to go.
func (s *Service) requireAnnounceChannel(ctx context.Context, guildID string) error {
	settings, err := s.store.GetGuildSettings(ctx, guildID)
	if err != nil {
		return err
	}
	if settings.AnnounceChannelID == "" {
		return ErrNoAnnounceChannel
	}
	return nil
}

// NewGame opens a game drawing from the given pools (admin only) and publishes
// GameOpened. name is an optional label (empty -> derived from the pool names).
func (s *Service) NewGame(ctx context.Context, guildID, userID, name string, poolIDs []int64, replace bool) (store.Game, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return store.Game{}, err
	}
	if err := s.requireAnnounceChannel(ctx, guildID); err != nil {
		return store.Game{}, err
	}
	game, err := s.store.NewGame(ctx, guildID, name, userID, poolIDs, replace)
	if err != nil {
		return store.Game{}, err
	}
	s.hub.Publish(events.Event{Kind: events.GameOpened, GuildID: guildID, GameID: game.ID, UserID: userID})
	return game, nil
}

// AbortGame aborts a specific open game (admin only).
func (s *Service) AbortGame(ctx context.Context, guildID, userID string, gameID int64) (store.Game, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return store.Game{}, err
	}
	game, err := s.store.GetGame(ctx, guildID, gameID)
	if err != nil {
		return store.Game{}, err
	}
	if err := s.store.AbortGame(ctx, guildID, game.ID); err != nil {
		return store.Game{}, err
	}
	s.hub.Publish(events.Event{Kind: events.GameAborted, GuildID: guildID, GameID: game.ID, UserID: userID})
	return game, nil
}

// DealCard returns the user's card for a specific game, dealing one if needed. Any
// guild member may deal in; the game must still be open.
func (s *Service) DealCard(ctx context.Context, guildID, userID string, gameID int64) (store.Card, store.Game, error) {
	game, err := s.store.GetGame(ctx, guildID, gameID)
	if err != nil {
		return store.Card{}, store.Game{}, err
	}
	var r *rand.Rand
	if s.rng != nil {
		r = s.rng()
	}
	card, err := s.store.GetOrDealCard(ctx, guildID, game.ID, userID, r)
	if err != nil {
		return store.Card{}, store.Game{}, err
	}
	s.hub.Publish(events.Event{Kind: events.CardDealt, GuildID: guildID, GameID: game.ID, CardID: card.ID, UserID: userID})
	return card, game, nil
}

// ToggleCell flips a cell on a card. The actor must own the card or be an admin.
// It returns the updated card and whether it now has a completed line.
func (s *Service) ToggleCell(ctx context.Context, guildID, actorUserID string, cardID int64, idx int) (store.Card, bool, error) {
	card, err := s.store.GetCard(ctx, guildID, cardID)
	if err != nil {
		return store.Card{}, false, err
	}
	allowed := card.UserID == actorUserID
	if !allowed {
		if admin, aerr := s.IsAdmin(ctx, guildID, actorUserID); aerr != nil {
			return store.Card{}, false, aerr
		} else if admin {
			allowed = true
		}
	}
	if !allowed {
		return store.Card{}, false, ErrForbidden
	}

	updated, hasBingo, err := s.store.ToggleCell(ctx, guildID, cardID, idx, true)
	if err != nil {
		return store.Card{}, false, err
	}
	s.hub.Publish(events.Event{Kind: events.CellToggled, GuildID: guildID, GameID: updated.GameID, CardID: cardID, UserID: actorUserID})
	return updated, hasBingo, nil
}

// CallBingo finalizes a win for the caller's card and publishes GameFinished.
func (s *Service) CallBingo(ctx context.Context, guildID, userID string, cardID int64) (store.CallBingoResult, error) {
	res, err := s.store.CallBingo(ctx, guildID, cardID, userID)
	if err != nil {
		return store.CallBingoResult{}, err
	}
	s.hub.Publish(events.Event{Kind: events.GameFinished, GuildID: guildID, GameID: res.Game.ID, CardID: cardID, UserID: userID})
	return res, nil
}
