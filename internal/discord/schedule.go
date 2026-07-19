package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/when"
)

// handleSchedule resolves the requested time, then shows an instance multi-select
// so one schedule can open several instances at once. The resolved fire time is
// carried in the select's custom id, keeping the flow stateless.
func (b *Bot) handleSchedule(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	if admin, err := b.svc.IsAdmin(ctx, i.GuildID, interactionUserID(i)); err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	} else if !admin {
		b.replyEphemeral(i, "Only bingo admins can schedule games.")
		return
	}

	fireAt, err := when.Resolve(time.Now(), optString(opts, "in"), optString(opts, "at"), optString(opts, "tz"))
	if err != nil {
		b.replyEphemeral(i, "Could not schedule: "+err.Error())
		return
	}
	replace := optBool(opts, "replace")

	options := make([]discordgo.SelectMenuOption, 0, len(bingo.Instances()))
	for _, inst := range bingo.Instances() {
		options = append(options, discordgo.SelectMenuOption{Label: inst.Label(), Value: string(inst)})
	}
	minSel := 1
	replaceFlag := "0"
	if replace {
		replaceFlag = "1"
	}
	b.respond(i, &discordgo.InteractionResponseData{
		Flags: discordgo.MessageFlagsEphemeral,
		Content: fmt.Sprintf("Games will open <t:%d:F> (<t:%d:R>). Pick which instances to open:",
			fireAt.Unix(), fireAt.Unix()),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:  discordgo.StringSelectMenu,
					CustomID:  fmt.Sprintf("sched:%d:%s", fireAt.Unix(), replaceFlag),
					MinValues: &minSel,
					MaxValues: len(options),
					Options:   options,
				},
			}},
		},
	})
}

// handleScheduleSelect records a schedule for each chosen instance.
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
	userID := interactionUserID(i)

	var scheduled, failed []string
	for _, val := range i.MessageComponentData().Values {
		inst, err := bingo.ParseInstance(val)
		if err != nil {
			continue
		}
		if _, err := b.svc.ScheduleGame(ctx, i.GuildID, userID, inst, fireUnix, replace); err != nil {
			failed = append(failed, inst.Label())
			continue
		}
		scheduled = append(scheduled, inst.Label())
	}

	var sb strings.Builder
	if len(scheduled) > 0 {
		fmt.Fprintf(&sb, "Scheduled for <t:%d:F>: %s.", fireUnix, strings.Join(scheduled, ", "))
		if replace {
			sb.WriteString(" Any game open at that time will be replaced.")
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(&sb, "\nCould not schedule: %s.", strings.Join(failed, ", "))
	}
	if sb.Len() == 0 {
		sb.WriteString("Nothing was scheduled.")
	}
	b.respondEditText(i, sb.String())
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
			repl = " (replaces any open game)"
		}
		fmt.Fprintf(&sb, "`%d` %s - <t:%d:F> (<t:%d:R>)%s\n", s.ID, s.Instance.Label(), s.FireAt, s.FireAt, repl)
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
				b.runDueSchedules(ctx)
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
