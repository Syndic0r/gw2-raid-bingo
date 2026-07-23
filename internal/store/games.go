package store

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// Game/card errors surfaced to callers for user-facing messages.
var (
	// ErrGameOpen is returned when opening a game whose exact pool-set already has
	// an open game and the caller did not ask to replace it.
	ErrGameOpen = errors.New("a game with these pools is already open")
	// ErrNoPoolsSelected is returned when opening a game with no pools chosen.
	ErrNoPoolsSelected = errors.New("select at least one pool to start a game")
	// ErrGameNotOpen is returned when acting on a game that is not open.
	ErrGameNotOpen = errors.New("game is not open")
	// ErrNoBingo is returned when a player calls bingo without a completed line.
	ErrNoBingo = errors.New("card has no completed line")
	// ErrCellFree is returned when trying to toggle the free centre.
	ErrCellFree = errors.New("the free centre cannot be toggled")
	// ErrForbidden is returned when the actor may not toggle a card.
	ErrForbidden = errors.New("not allowed")
)

// NewGame opens a game drawing from the given set of pools. name is an optional
// human label; when empty it is derived from the pool names. The pool set is the
// game's identity: at most one open game per distinct set per guild. If a game is
// already open for the same set and replace is false, it returns ErrGameOpen; if
// replace is true, that open game is aborted first (its cards become read-only).
//
// It validates that at least one pool is selected (ErrNoPoolsSelected), that every
// pool belongs to the guild, and that the pools jointly hold enough distinct active
// squares to fill a card - all before inserting, so no undealable game is created.
func (s *Store) NewGame(ctx context.Context, guildID, name, createdBy string, poolIDs []int64, replace bool) (Game, error) {
	cleanName, err := cleanGameName(name)
	if err != nil {
		return Game{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Game{}, err
	}
	defer tx.Rollback()

	// Validate + dedupe the pools (in the caller's order), rejecting an empty
	// selection and any pool that is not this guild's.
	pools, err := s.validatePoolsTx(ctx, tx, guildID, poolIDs)
	if err != nil {
		return Game{}, err
	}
	if len(pools) == 0 {
		return Game{}, ErrNoPoolsSelected
	}
	ids := make([]int64, len(pools))
	for i, p := range pools {
		ids[i] = p.ID
	}

	// The pools must jointly offer enough distinct squares to fill a board, checked
	// now so the failure is at game creation rather than the first card deal.
	entries, err := s.entriesForPoolsTx(ctx, tx, guildID, ids)
	if err != nil {
		return Game{}, err
	}
	if have := bingo.UsableEntryCount(toBingoEntries(entries)); have < bingo.FillCount {
		return Game{}, validationErr(
			"the selected pools have only %d unique squares; a game needs at least %d - add more squares or select more pools",
			have, bingo.FillCount)
	}

	key := poolSetKey(ids)

	var openID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM games WHERE guild_id = ? AND pool_set_key = ? AND status = ?`,
		guildID, key, StatusOpen).Scan(&openID)
	switch {
	case err == nil:
		if !replace {
			return Game{}, ErrGameOpen
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE games SET status = ?, finished_at = ? WHERE id = ?`,
			StatusAborted, now(), openID); err != nil {
			return Game{}, err
		}
	case errors.Is(err, sql.ErrNoRows):
		// no open game for this set - fine
	default:
		return Game{}, err
	}

	if cleanName == "" {
		cleanName = deriveGameName(pools)
	}
	poolJSON, err := json.Marshal(ids)
	if err != nil {
		return Game{}, err
	}

	ts := now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO games (guild_id, name, pool_set_key, status, created_by, pool_ids, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		guildID, cleanName, key, StatusOpen, createdBy, string(poolJSON), ts)
	if err != nil {
		if isUniqueViolation(err) {
			return Game{}, ErrGameOpen
		}
		return Game{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Game{}, err
	}
	if err := tx.Commit(); err != nil {
		return Game{}, err
	}
	return Game{
		ID: id, GuildID: guildID, Name: cleanName, PoolSetKey: key, Status: StatusOpen,
		CreatedBy: createdBy, PoolIDs: ids, CreatedAt: ts,
	}, nil
}

// validatePoolsTx dedupes ids (preserving the caller's order) and verifies each
// pool belongs to the guild, returning the resolved pool rows. A pool id that is
// unknown or from another guild is rejected rather than silently dropped.
func (s *Store) validatePoolsTx(ctx context.Context, tx *sql.Tx, guildID string, ids []int64) ([]Pool, error) {
	out := make([]Pool, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		var p Pool
		err := tx.QueryRowContext(ctx,
			`SELECT id, guild_id, slug, name, created_at FROM pools WHERE id = ? AND guild_id = ?`,
			id, guildID).Scan(&p.ID, &p.GuildID, &p.Slug, &p.Name, &p.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, validationErr("pool %d does not belong to this server", id)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// poolSetKey is the canonical identity of a set of pool ids: sorted ascending,
// de-duplicated, and joined with commas. Two selections of the same pools produce
// the same key regardless of order, which is what the one-open-game-per-set unique
// index keys on.
func poolSetKey(ids []int64) string {
	uniq := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	sort.Slice(uniq, func(i, j int) bool { return uniq[i] < uniq[j] })
	parts := make([]string, len(uniq))
	for i, id := range uniq {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

// deriveGameName builds a default label from the pool names (in selection order),
// rune-capped to MaxGameNameLen so a long selection cannot produce an oversized name.
func deriveGameName(pools []Pool) string {
	names := make([]string, len(pools))
	for i, p := range pools {
		names[i] = p.Name
	}
	name := strings.Join(names, " + ")
	if utf8.RuneCountInString(name) > MaxGameNameLen {
		r := []rune(name)
		name = strings.TrimSpace(string(r[:MaxGameNameLen-1])) + "…"
	}
	return name
}

// AbortGame marks an open game aborted; its cards become read-only history.
func (s *Store) AbortGame(ctx context.Context, guildID string, gameID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE games SET status = ?, finished_at = ? WHERE id = ? AND guild_id = ? AND status = ?`,
		StatusAborted, now(), gameID, guildID, StatusOpen)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrGameNotOpen
	}
	return nil
}

// gameColumns is the shared SELECT list for scanning a Game.
const gameColumns = `id, guild_id, name, pool_set_key, status, created_by, pool_ids, created_at,
	        finished_at, winner_user_id, winner_card_id`

// ListOpenGames returns a guild's currently-open games, newest first.
func (s *Store) ListOpenGames(ctx context.Context, guildID string) ([]Game, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+gameColumns+` FROM games WHERE guild_id = ? AND status = ?
		 ORDER BY created_at DESC, id DESC`, guildID, StatusOpen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGames(rows)
}

// ListRecentGames returns a guild's most recent finished or aborted games,
// newest first, for the history view.
func (s *Store) ListRecentGames(ctx context.Context, guildID string, limit int) ([]Game, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+gameColumns+` FROM games WHERE guild_id = ? AND status IN (?, ?)
		 ORDER BY COALESCE(finished_at, created_at) DESC, id DESC LIMIT ?`,
		guildID, StatusFinished, StatusAborted, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGames(rows)
}

func scanGames(rows *sql.Rows) ([]Game, error) {
	var out []Game
	for rows.Next() {
		g, err := scanGameRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// scanGameRows scans a game from a multi-row query.
func scanGameRows(rows *sql.Rows) (Game, error) {
	var (
		g            Game
		poolJSON     string
		finishedAt   sql.NullInt64
		winnerUserID sql.NullString
		winnerCardID sql.NullInt64
	)
	if err := rows.Scan(&g.ID, &g.GuildID, &g.Name, &g.PoolSetKey, &g.Status, &g.CreatedBy, &poolJSON,
		&g.CreatedAt, &finishedAt, &winnerUserID, &winnerCardID); err != nil {
		return Game{}, err
	}
	g.FinishedAt = finishedAt.Int64
	g.WinnerUserID = winnerUserID.String
	g.WinnerCardID = winnerCardID.Int64
	if poolJSON != "" {
		if err := json.Unmarshal([]byte(poolJSON), &g.PoolIDs); err != nil {
			return Game{}, err
		}
	}
	return g, nil
}

// GetGame returns a game by id within a guild, or ErrNotFound.
func (s *Store) GetGame(ctx context.Context, guildID string, gameID int64) (Game, error) {
	return s.scanGame(s.db.QueryRowContext(ctx,
		`SELECT `+gameColumns+` FROM games WHERE guild_id = ? AND id = ?`, guildID, gameID))
}

func (s *Store) scanGame(row *sql.Row) (Game, error) {
	var (
		g            Game
		poolJSON     string
		finishedAt   sql.NullInt64
		winnerUserID sql.NullString
		winnerCardID sql.NullInt64
	)
	err := row.Scan(&g.ID, &g.GuildID, &g.Name, &g.PoolSetKey, &g.Status, &g.CreatedBy, &poolJSON,
		&g.CreatedAt, &finishedAt, &winnerUserID, &winnerCardID)
	if errors.Is(err, sql.ErrNoRows) {
		return Game{}, ErrNotFound
	}
	if err != nil {
		return Game{}, err
	}
	g.FinishedAt = finishedAt.Int64
	g.WinnerUserID = winnerUserID.String
	g.WinnerCardID = winnerCardID.Int64
	if poolJSON != "" {
		if err := json.Unmarshal([]byte(poolJSON), &g.PoolIDs); err != nil {
			return Game{}, err
		}
	}
	return g, nil
}

// GetOrDealCard returns the user's card for a game, dealing a new one if they
// have none. Dealing requires the game to be open and the pools to hold enough
// distinct entries; otherwise it returns bingo.ErrNotEnoughEntries. r supplies
// randomness (nil uses a fresh source seeded from the game and user).
func (s *Store) GetOrDealCard(ctx context.Context, guildID string, gameID int64, userID string, r *rand.Rand) (Card, error) {
	if existing, err := s.getUserCard(ctx, s.db, gameID, userID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Card{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback()

	game, err := s.scanGame(tx.QueryRowContext(ctx,
		`SELECT `+gameColumns+` FROM games WHERE guild_id = ? AND id = ?`, guildID, gameID))
	if err != nil {
		return Card{}, err
	}
	if game.Status != StatusOpen {
		return Card{}, ErrGameNotOpen
	}

	// Draw from the union of the game's selected pools. A pool id whose pool was
	// deleted after the game opened simply contributes no entries; if that drops
	// the union below a full card, GenerateCard returns bingo.ErrNotEnoughEntries
	// rather than panicking.
	entries, err := s.entriesForPoolsTx(ctx, tx, guildID, game.PoolIDs)
	if err != nil {
		return Card{}, err
	}

	if r == nil {
		r = rand.New(rand.NewSource(cryptoSeed()))
	}
	card, err := bingo.GenerateCard(toBingoEntries(entries), r)
	if err != nil {
		return Card{}, err
	}

	ts := now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO cards (game_id, guild_id, user_id, created_at) VALUES (?, ?, ?, ?)`,
		gameID, guildID, userID, ts)
	if err != nil {
		if isUniqueViolation(err) {
			// Concurrent deal for the same user; return the winner of the race.
			return s.getUserCard(ctx, s.db, gameID, userID)
		}
		return Card{}, err
	}
	cardID, err := res.LastInsertId()
	if err != nil {
		return Card{}, err
	}
	for _, c := range card.Cells {
		var entryID any
		if !c.Free && c.EntryID != 0 {
			entryID = c.EntryID
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO card_cells (card_id, idx, entry_id, text, free, marked)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			cardID, c.Index, entryID, c.Text, boolInt(c.Free), boolInt(c.Free)); err != nil {
			return Card{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	return s.getUserCard(ctx, s.db, gameID, userID)
}

// ToggleCell flips a cell's marked state. Only the card owner or a bingo admin
// may toggle, the game must be open, and the free centre is immutable. It
// returns the updated card and whether it now has a completed line.
func (s *Store) ToggleCell(ctx context.Context, guildID string, cardID int64, idx int, isOwnerOrAdmin bool) (Card, bool, error) {
	if !isOwnerOrAdmin {
		return Card{}, false, ErrForbidden
	}
	if idx < 0 || idx >= bingo.CellCount {
		return Card{}, false, validationErr("cell index out of range")
	}
	if idx == bingo.CenterIdx {
		return Card{}, false, ErrCellFree
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Card{}, false, err
	}
	defer tx.Rollback()

	var status string
	if err := tx.QueryRowContext(ctx,
		`SELECT g.status FROM cards c JOIN games g ON g.id = c.game_id
		 WHERE c.id = ? AND c.guild_id = ?`, cardID, guildID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Card{}, false, ErrNotFound
		}
		return Card{}, false, err
	}
	if status != StatusOpen {
		return Card{}, false, ErrGameNotOpen
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE card_cells SET marked = 1 - marked WHERE card_id = ? AND idx = ? AND free = 0`,
		cardID, idx)
	if err != nil {
		return Card{}, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Card{}, false, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return Card{}, false, err
	}

	card, err := s.getCardByID(ctx, s.db, guildID, cardID)
	if err != nil {
		return Card{}, false, err
	}
	return card, bingo.HasBingo(card.Marks()), nil
}

// CallBingoResult reports the outcome of a call.
type CallBingoResult struct {
	Game Game // the finished game
	Card Card // the winning card
}

// CallBingo finalizes a win. It verifies the card has a completed line and, in a
// single transaction, transitions the game to finished with this card's owner as
// the winner - but only if the game is still open, so the first caller wins any
// race and later callers get ErrGameNotOpen.
func (s *Store) CallBingo(ctx context.Context, guildID string, cardID int64, userID string) (CallBingoResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CallBingoResult{}, err
	}
	defer tx.Rollback()

	card, err := s.getCardByID(ctx, tx, guildID, cardID)
	if err != nil {
		return CallBingoResult{}, err
	}
	if card.UserID != userID {
		return CallBingoResult{}, ErrForbidden
	}
	if !bingo.HasBingo(card.Marks()) {
		return CallBingoResult{}, ErrNoBingo
	}

	ts := now()
	res, err := tx.ExecContext(ctx,
		`UPDATE games SET status = ?, finished_at = ?, winner_user_id = ?, winner_card_id = ?
		 WHERE id = ? AND guild_id = ? AND status = ?`,
		StatusFinished, ts, userID, cardID, card.GameID, guildID, StatusOpen)
	if err != nil {
		return CallBingoResult{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CallBingoResult{}, ErrGameNotOpen // already finished/aborted, or lost the race
	}
	if err := tx.Commit(); err != nil {
		return CallBingoResult{}, err
	}

	game, err := s.GetGame(ctx, guildID, card.GameID)
	if err != nil {
		return CallBingoResult{}, err
	}
	return CallBingoResult{Game: game, Card: card}, nil
}

// ListCards returns every card in a game with its cells, for admin inspection
// and status displays.
func (s *Store) ListCards(ctx context.Context, guildID string, gameID int64) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, game_id, guild_id, user_id, created_at
		 FROM cards WHERE guild_id = ? AND game_id = ? ORDER BY created_at, id`, guildID, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.GameID, &c.GuildID, &c.UserID, &c.CreatedAt); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range cards {
		cells, err := s.cardCells(ctx, s.db, cards[i].ID)
		if err != nil {
			return nil, err
		}
		cards[i].Cells = cells
	}
	return cards, nil
}

// GetCard returns a single card with cells by id within a guild.
func (s *Store) GetCard(ctx context.Context, guildID string, cardID int64) (Card, error) {
	return s.getCardByID(ctx, s.db, guildID, cardID)
}

// GetUserCard returns a user's card for a game, or ErrNotFound.
func (s *Store) GetUserCard(ctx context.Context, gameID int64, userID string) (Card, error) {
	return s.getUserCard(ctx, s.db, gameID, userID)
}

// --- internal helpers over an arbitrary querier (db or tx) ---

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) getUserCard(ctx context.Context, q querier, gameID int64, userID string) (Card, error) {
	var c Card
	err := q.QueryRowContext(ctx,
		`SELECT id, game_id, guild_id, user_id, created_at
		 FROM cards WHERE game_id = ? AND user_id = ?`, gameID, userID).
		Scan(&c.ID, &c.GameID, &c.GuildID, &c.UserID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}
	cells, err := s.cardCells(ctx, q, c.ID)
	if err != nil {
		return Card{}, err
	}
	c.Cells = cells
	return c, nil
}

func (s *Store) getCardByID(ctx context.Context, q querier, guildID string, cardID int64) (Card, error) {
	var c Card
	err := q.QueryRowContext(ctx,
		`SELECT id, game_id, guild_id, user_id, created_at
		 FROM cards WHERE id = ? AND guild_id = ?`, cardID, guildID).
		Scan(&c.ID, &c.GameID, &c.GuildID, &c.UserID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}
	cells, err := s.cardCells(ctx, q, c.ID)
	if err != nil {
		return Card{}, err
	}
	c.Cells = cells
	return c, nil
}

func (s *Store) cardCells(ctx context.Context, q querier, cardID int64) ([]CardCell, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT idx, entry_id, text, free, marked FROM card_cells WHERE card_id = ? ORDER BY idx`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cells := make([]CardCell, 0, bingo.CellCount)
	for rows.Next() {
		var (
			cell    CardCell
			entryID sql.NullInt64
		)
		if err := rows.Scan(&cell.Index, &entryID, &cell.Text, &cell.Free, &cell.Marked); err != nil {
			return nil, err
		}
		cell.EntryID = entryID.Int64
		cells = append(cells, cell)
	}
	return cells, rows.Err()
}

func (s *Store) entriesForPoolsTx(ctx context.Context, tx *sql.Tx, guildID string, poolIDs []int64) ([]Entry, error) {
	if len(poolIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(poolIDs))
	args := make([]any, 0, len(poolIDs)+1)
	args = append(args, guildID)
	for i, id := range poolIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := `SELECT id, guild_id, pool_id, text, active, created_at, updated_at
	      FROM entries WHERE guild_id = ? AND active = 1 AND pool_id IN (` + strings.Join(placeholders, ", ") + `) ORDER BY id`
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.GuildID, &e.PoolID, &e.Text, &e.Active, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cryptoSeed seeds the dealing RNG from crypto/rand so a player cannot predict
// their card layout in advance (the previous seed was derived from the game and
// user ids, which are both visible). Tests keep determinism by passing their own
// *rand.Rand.
func cryptoSeed() int64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively fatal elsewhere; fall back to a
		// unique-ish value rather than a constant.
		return int64(time.Now().UnixNano())
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}
