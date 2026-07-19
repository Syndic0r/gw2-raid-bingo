package discord

import (
	"bytes"
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// replyEphemeral sends a private text reply to an interaction.
func (b *Bot) replyEphemeral(i *discordgo.InteractionCreate, msg string) {
	b.respond(i, &discordgo.InteractionResponseData{
		Content: msg,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}

func (b *Bot) replyEphemeralf(i *discordgo.InteractionCreate, format string, args ...any) {
	b.replyEphemeral(i, fmt.Sprintf(format, args...))
}

// replyPublic sends a normal (visible) text reply.
func (b *Bot) replyPublic(i *discordgo.InteractionCreate, msg string) {
	b.respond(i, &discordgo.InteractionResponseData{Content: msg})
}

func (b *Bot) respond(i *discordgo.InteractionCreate, data *discordgo.InteractionResponseData) {
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	})
	if err != nil {
		b.log.Printf("respond: %v", err)
	}
}

// respondCardView responds with an ephemeral interactive card (image + buttons).
func (b *Bot) respondCardView(i *discordgo.InteractionCreate, v cardView) {
	data, err := v.responseData()
	if err != nil {
		b.log.Printf("card view: %v", err)
		b.replyEphemeral(i, "Sorry, the card image could not be rendered.")
		return
	}
	data.Flags = discordgo.MessageFlagsEphemeral
	if err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	}); err != nil {
		b.log.Printf("respond card: %v", err)
	}
}

// updateCardView edits the component message in place with a fresh card.
func (b *Bot) updateCardView(i *discordgo.InteractionCreate, v cardView) {
	data, err := v.responseData()
	if err != nil {
		b.log.Printf("card view: %v", err)
		return
	}
	data.Flags = discordgo.MessageFlagsEphemeral
	if err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: data,
	}); err != nil {
		b.log.Printf("update card: %v", err)
	}
}

// --- option parsing helpers ---

// subcommand returns the invoked subcommand's name and its options.
func subcommand(i *discordgo.InteractionCreate) (string, []*discordgo.ApplicationCommandInteractionDataOption) {
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return "", nil
	}
	return opts[0].Name, opts[0].Options
}

func optString(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range opts {
		if o.Name == name {
			return o.StringValue()
		}
	}
	return ""
}

func optBool(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) bool {
	for _, o := range opts {
		if o.Name == name {
			return o.BoolValue()
		}
	}
	return false
}

func optInt(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) int64 {
	for _, o := range opts {
		if o.Name == name {
			return o.IntValue()
		}
	}
	return 0
}

// optAttachment resolves an attachment option to its uploaded file.
func optAttachment(i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption, name string) *discordgo.MessageAttachment {
	for _, o := range opts {
		if o.Name == name && o.Type == discordgo.ApplicationCommandOptionAttachment {
			// comma-ok: a malformed payload must not panic (a panic in a
			// discordgo handler would take the whole process down).
			id, ok := o.Value.(string)
			if !ok {
				return nil
			}
			resolved := i.ApplicationCommandData().Resolved
			if resolved == nil {
				return nil
			}
			return resolved.Attachments[id]
		}
	}
	return nil
}

// interactionUserID returns the acting user's id whether the interaction came
// from a guild member (commands/components in a guild) or a DM.
func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// fileFromBytes builds a discordgo file attachment.
func fileFromBytes(name, contentType string, data []byte) *discordgo.File {
	return &discordgo.File{Name: name, ContentType: contentType, Reader: bytes.NewReader(data)}
}
