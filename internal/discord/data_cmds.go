package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

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
		pool, err := b.svc.CreatePool(ctx, i.GuildID, userID, slug, name)
		if err != nil {
			b.replyEphemeral(i, b.describeError(err))
			return
		}
		b.replyEphemeralf(i, "Created pool **%s** (`%s`). Add squares with `/bingo-data add pool:%s`.", pool.Name, pool.Slug, pool.Slug)
	case "add":
		b.handleDataAdd(ctx, i, opts, userID)
	case "list":
		b.handleDataList(ctx, i, opts)
	case "clear":
		b.handleDataClear(ctx, i, opts)
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

// resolvePool resolves a pool reference (slug) to a pool.
func (b *Bot) resolvePool(ctx context.Context, guildID, ref string) (store.Pool, error) {
	return b.svc.Store().GetPool(ctx, guildID, strings.ToLower(strings.TrimSpace(ref)))
}

func (b *Bot) handleDataAdd(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption, userID string) {
	pool, err := b.resolvePool(ctx, i.GuildID, optString(opts, "pool"))
	if err != nil {
		b.replyEphemeral(i, "No such pool. Pick one from the dropdown (or create it with `/bingo-data pool-add`).")
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
func (b *Bot) handleDataClear(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can clear a pool.") {
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
	// /bingo-data manages a guild's square library; reading it is admin-only like
	// every other subcommand in the group (players see squares on dealt cards).
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can view pool data.") {
		return
	}
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
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can import data.") {
		return
	}
	// Only fetch from Discord's CDN: the URL rides in the interaction payload,
	// and the bot must never be usable as a proxy to arbitrary hosts (SSRF).
	if !isDiscordCDNURL(att.URL) {
		b.replyEphemeral(i, "That attachment URL is not a Discord CDN URL.")
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
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can export pool data.") {
		return
	}
	// Export every pool under the `shared` bucket (the import side treats both
	// buckets as ordinary pools; a single bucket keeps the file simplest).
	out := store.SeedData{Shared: map[string][]string{}}
	pools, err := b.svc.Store().ListPools(ctx, i.GuildID)
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
		out.Shared[p.Slug] = texts
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

// isDiscordCDNURL reports whether u is an https URL on Discord's CDN hosts.
func isDiscordCDNURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "cdn.discordapp.com" || host == "media.discordapp.net"
}

// importDownloadClient fetches import attachments. Its CheckRedirect re-validates
// every hop against the Discord-CDN allowlist: the caller only checks the INITIAL
// URL, so without this a CDN URL that 3xx-redirects elsewhere (an SSRF vector)
// would be followed by the default client. Redirects are also capped.
var importDownloadClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if !isDiscordCDNURL(req.URL.String()) {
			return fmt.Errorf("refusing redirect to non-Discord-CDN host %q", req.URL.Host)
		}
		return nil
	},
}

// download fetches up to limit bytes from an attachment URL.
func (b *Bot) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := importDownloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
