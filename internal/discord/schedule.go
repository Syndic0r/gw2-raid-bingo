package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/when"
)

// handleSchedule resolves the requested time, then shows a pool multi-select so
// the schedule opens a game from the chosen pools when it fires. The resolved fire
// time and replace flag ride in the select's custom id, keeping the flow stateless.
func (b *Bot) handleSchedule(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can schedule games.") {
		return
	}

	fireAt, err := when.Resolve(time.Now(), optString(opts, "in"), optString(opts, "at"), optString(opts, "tz"))
	if err != nil {
		b.replyEphemeral(i, "Could not schedule: "+err.Error())
		return
	}
	pools, err := b.svc.Store().ListPools(ctx, i.GuildID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if len(pools) == 0 {
		b.replyEphemeral(i, "This server has no pools yet. Add squares with `/bingo-data` first.")
		return
	}
	replaceFlag := "0"
	if optBool(opts, "replace") {
		replaceFlag = "1"
	}
	b.respond(i, poolSelectData(fmt.Sprintf("sched:%d:%s", fireAt.Unix(), replaceFlag),
		fmt.Sprintf("A game will open <t:%d:F> (<t:%d:R>). Pick the pools to build it from:", fireAt.Unix(), fireAt.Unix()),
		pools))
}

// handleScheduleSelect records a schedule for the chosen pool-set.
func (b *Bot) handleScheduleSelect(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	parts := parseIDArgs(id) // sched:<fireUnix>:<replaceFlag>
	if len(parts) != 3 {
		b.respondEditText(i, "Invalid schedule control.")
		return
	}
	fireUnix, ok := atoi64(parts[1])
	if !ok {
		b.respondEditText(i, "Invalid schedule time.")
		return
	}
	replace := parts[2] == "1"

	var poolIDs []int64
	for _, v := range i.MessageComponentData().Values {
		if pid, ok := atoi64(v); ok {
			poolIDs = append(poolIDs, pid)
		}
	}
	if _, err := b.svc.ScheduleGame(ctx, i.GuildID, interactionUserID(i), "", poolIDs, fireUnix, replace); err != nil {
		b.respondEditText(i, b.describeError(err))
		return
	}
	msg := fmt.Sprintf("Scheduled a game with %d pool(s) for <t:%d:F>.", len(poolIDs), fireUnix)
	if replace {
		msg += " Any game with the same pools open at that time will be replaced."
	}
	b.respondEditText(i, msg)
}

// handleScheduled lists a guild's pending schedules.
func (b *Bot) handleScheduled(ctx context.Context, i *discordgo.InteractionCreate) {
	list, err := b.svc.ListScheduled(ctx, i.GuildID, interactionUserID(i))
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if len(list) == 0 {
		b.replyEphemeral(i, "No games are scheduled.")
		return
	}
	var sb strings.Builder
	sb.WriteString("**Scheduled games:**\n")
	for _, s := range list {
		repl := ""
		if s.ReplaceOpen {
			repl = " (replaces any open game with the same pools)"
		}
		label := s.Name
		if label == "" {
			label = fmt.Sprintf("%d pool(s)", len(s.PoolIDs))
		}
		fmt.Fprintf(&sb, "`%d` %s - <t:%d:F> (<t:%d:R>)%s\n", s.ID, label, s.FireAt, s.FireAt, repl)
	}
	sb.WriteString("\nCancel one with `/bingo unschedule id:<number>`.")
	b.replyEphemeral(i, sb.String())
}

// handleUnschedule cancels a pending schedule.
func (b *Bot) handleUnschedule(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	id := optInt(opts, "id")
	if err := b.svc.CancelScheduled(ctx, i.GuildID, interactionUserID(i), id); err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.replyEphemeralf(i, "Cancelled scheduled game #%d.", id)
}

// startScheduler runs the background loop that opens due scheduled games. It
// ticks every interval until ctx is cancelled.
func (b *Bot) startScheduler(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				func() {
					defer b.recoverGuard("scheduler tick")
					b.runDueSchedules(ctx)
				}()
			}
		}
	}()
}

func (b *Bot) runDueSchedules(ctx context.Context) {
	// RunDueSchedules opens each due game and publishes a GameOpened event; the
	// event bridge (onEvent) then refreshes the status message and posts the
	// game-open announcement, so nothing more is needed here.
	if _, err := b.svc.RunDueSchedules(ctx, time.Now().Unix()); err != nil {
		b.log.Printf("scheduler: %v", err)
	}
}
