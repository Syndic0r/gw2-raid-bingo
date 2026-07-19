package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// maxImportBytes caps an uploaded import file.
const maxImportBytes = 512 * 1024

// handleData dispatches /bingo-data subcommands. All are admin-gated in the
// service layer; a friendly forbidden message is shown on denial.
func (b *Bot) handleData(ctx context.Context, i *discordgo.InteractionCreate) {
	sub, opts := subcommand(i)
	userID := interactionUserID(i)
	switch sub {
	case "pool-add":
		slug := optString(opts, "slug")
		name := optString(opts, "name")
		pool, err := b.svc.CreateSharedPool(ctx, i.GuildID, userID, slug, name)
		if err != nil {
			b.replyEphemeral(i, b.describeError(err))
			return
		}
		b.replyEphemeralf(i, "Created shared pool **%s** (`%s`). Add squares with `/bingo-data add pool:%s`.", pool.Name, pool.Slug, pool.Slug)
	case "add":
		b.handleDataAdd(ctx, i, opts, userID)
	case "list":
		b.handleDataList(ctx, i, opts)
	case "clear":
		b.handleDataClear(ctx, i, opts, userID)
	case "remove":
		b.handleDataRemove(ctx, i, opts, userID)
	case "import":
		b.handleDataImport(ctx, i, opts, userID)
	case "export":
		b.handleDataExport(ctx, i)
	default:
		b.replyEphemeral(i, "Unknown subcommand.")
	}
}

// resolvePool resolves a pool reference (instance key or shared slug) to a pool.
func (b *Bot) resolvePool(ctx context.Context, guildID, ref string) (store.Pool, error) {
	ref = strings.ToLower(strings.TrimSpace(ref))
	if inst, err := bingo.ParseInstance(ref); err == nil {
		return b.svc.Store().InstancePool(ctx, guildID, inst)
	}
	return b.svc.Store().GetPool(ctx, guildID, store.KindShared, ref)
}

func (b *Bot) handleDataAdd(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption, userID string) {
	pool, err := b.resolvePool(ctx, i.GuildID, optString(opts, "pool"))
	if err != nil {
		b.replyEphemeral(i, "No such pool. Use an instance key (w1..htcm) or a shared pool slug.")
		return
	}
	entry, err := b.svc.AddEntry(ctx, i.GuildID, userID, pool.ID, optString(opts, "text"))
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.replyEphemeralf(i, "Added square #%d to **%s**.", entry.ID, pool.Name)
}

// handleDataRemove deletes the square chosen from the searchable dropdown. The
// "square" value is the entry id the autocomplete supplied.
func (b *Bot) handleDataRemove(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption, userID string) {
	entryID, ok := atoi64(optString(opts, "square"))
	if !ok {
		b.replyEphemeral(i, "Pick a square from the dropdown - start typing to search within the pool.")
		return
	}
	if err := b.svc.RemoveEntry(ctx, i.GuildID, userID, entryID); err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.replyEphemeral(i, "Removed that square.")
}

// handleDataClear asks to confirm emptying a whole pool, then the confirm button
// (handleClearPool) does it.
func (b *Bot) handleDataClear(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption, userID string) {
	if admin, err := b.svc.IsAdmin(ctx, i.GuildID, userID); err != nil || !admin {
		b.replyEphemeral(i, "Only bingo admins can clear a pool.")
		return
	}
	pool, err := b.resolvePool(ctx, i.GuildID, optString(opts, "pool"))
	if err != nil {
		b.replyEphemeral(i, "No such pool. Pick one from the dropdown.")
		return
	}
	entries, err := b.svc.Store().ListEntries(ctx, i.GuildID, pool.ID, true)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if len(entries) == 0 {
		b.replyEphemeralf(i, "**%s** is already empty.", pool.Name)
		return
	}
	b.respond(i, &discordgo.InteractionResponseData{
		Flags:   discordgo.MessageFlagsEphemeral,
		Content: fmt.Sprintf("Remove all **%d** squares from **%s**? This can't be undone.", len(entries), pool.Name),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Clear pool", Style: discordgo.DangerButton, CustomID: fmt.Sprintf("clearpool:%d", pool.ID)},
			}},
		},
	})
}

func (b *Bot) handleDataList(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	pool, err := b.resolvePool(ctx, i.GuildID, optString(opts, "pool"))
	if err != nil {
		b.replyEphemeral(i, "No such pool.")
		return
	}
	entries, err := b.svc.Store().ListEntries(ctx, i.GuildID, pool.ID, true)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if len(entries) == 0 {
		b.replyEphemeralf(i, "**%s** has no squares yet.", pool.Name)
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "**%s** (%d squares):\n", pool.Name, len(entries))
	for _, e := range entries {
		line := fmt.Sprintf("`%d` %s\n", e.ID, e.Text)
		if sb.Len()+len(line) > 1900 { // Discord message limit headroom
			sb.WriteString("... (list truncated)\n")
			break
		}
		sb.WriteString(line)
	}
	b.replyEphemeral(i, sb.String())
}

func (b *Bot) handleDataImport(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption, userID string) {
	att := optAttachment(i, opts, "file")
	if att == nil {
		b.replyEphemeral(i, "Attach a JSON file to import.")
		return
	}
	// Verify permission before downloading anything.
	if admin, err := b.svc.IsAdmin(ctx, i.GuildID, userID); err != nil || !admin {
		b.replyEphemeral(i, "Only bingo admins can import data.")
		return
	}
	data, err := b.download(ctx, att.URL, maxImportBytes)
	if err != nil {
		b.replyEphemeral(i, "Could not download the attachment.")
		return
	}
	parsed, err := store.ParseSeed(strings.NewReader(string(data)))
	if err != nil {
		b.replyEphemeral(i, "That file is not valid import JSON. Expected the /bingo-data export format.")
		return
	}
	res, err := b.svc.ImportData(ctx, i.GuildID, userID, parsed)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.replyEphemeralf(i, "Imported %d squares (%d new pools, %d rows skipped).", res.Inserted, res.PoolsMade, res.SkippedRows)
}

func (b *Bot) handleDataExport(ctx context.Context, i *discordgo.InteractionCreate) {
	out := store.SeedData{
		Instance: map[string][]string{},
		Shared:   map[string][]string{},
	}
	for _, kind := range []string{store.KindInstance, store.KindShared} {
		pools, err := b.svc.Store().ListPools(ctx, i.GuildID, kind)
		if err != nil {
			b.replyEphemeral(i, b.describeError(err))
			return
		}
		for _, p := range pools {
			entries, err := b.svc.Store().ListEntries(ctx, i.GuildID, p.ID, true)
			if err != nil {
				b.replyEphemeral(i, b.describeError(err))
				return
			}
			texts := make([]string, 0, len(entries))
			for _, e := range entries {
				texts = append(texts, e.Text)
			}
			if kind == store.KindInstance {
				out.Instance[p.Slug] = texts
			} else {
				out.Shared[p.Slug] = texts
			}
		}
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		b.replyEphemeral(i, "Could not build the export.")
		return
	}
	b.respond(i, &discordgo.InteractionResponseData{
		Flags:   discordgo.MessageFlagsEphemeral,
		Content: "Here is this server's bingo data. Re-import it with `/bingo-data import`.",
		Files:   []*discordgo.File{fileFromBytes("bingo-data.json", "application/json", payload)},
	})
}

// download fetches up to limit bytes from an attachment URL.
func (b *Bot) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
