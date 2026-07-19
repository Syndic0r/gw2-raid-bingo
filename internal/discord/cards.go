package discord

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/render"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// cardView renders a card as an image plus, unless read-only, a 5x5 grid of
// toggle buttons. The grid fills Discord's five-action-row limit exactly, so the
// CALL BINGO control is delivered as a separate follow-up when a line completes
// (see maybePromptBingo).
type cardView struct {
	title    string
	subtitle string
	card     store.Card
	readOnly bool
}

func (v cardView) responseData() (*discordgo.InteractionResponseData, error) {
	cells := make([]render.Cell, bingo.CellCount)
	for _, c := range v.card.Cells {
		if c.Index >= 0 && c.Index < bingo.CellCount {
			cells[c.Index] = render.Cell{Text: c.Text, Marked: c.Marked, Free: c.Free}
		}
	}
	png, err := render.RenderCard(render.Options{Title: v.title, Subtitle: v.subtitle, Cells: cells})
	if err != nil {
		return nil, err
	}
	// Clear any prior attachment so editing the message REPLACES the card image
	// instead of stacking a new one on every toggle.
	noAttachments := []*discordgo.MessageAttachment{}
	data := &discordgo.InteractionResponseData{
		Files:       []*discordgo.File{fileFromBytes("bingo.png", "image/png", png)},
		Attachments: &noAttachments,
	}
	if !v.readOnly {
		data.Components = gridComponents(v.card, bingo.HasBingo(v.card.Marks()))
	}
	return data, nil
}

// gridComponents builds the five action rows of toggle buttons. The grid fills
// Discord's five-row limit, so there is no room for a separate CALL BINGO button;
// instead, once a line is complete the (otherwise inert) free centre button turns
// into the green CALL BINGO button, right on the card.
func gridComponents(card store.Card, hasBingo bool) []discordgo.MessageComponent {
	marked := make([]bool, bingo.CellCount)
	free := make([]bool, bingo.CellCount)
	for _, c := range card.Cells {
		if c.Index >= 0 && c.Index < bingo.CellCount {
			marked[c.Index] = c.Marked
			free[c.Index] = c.Free
		}
	}
	rows := make([]discordgo.MessageComponent, 0, bingo.Size)
	for r := 0; r < bingo.Size; r++ {
		buttons := make([]discordgo.MessageComponent, 0, bingo.Size)
		for col := 0; col < bingo.Size; col++ {
			idx := r*bingo.Size + col
			if free[idx] {
				if hasBingo {
					buttons = append(buttons, discordgo.Button{
						Label:    "BINGO!",
						Emoji:    &discordgo.ComponentEmoji{Name: "🎉"},
						Style:    discordgo.SuccessButton,
						CustomID: fmt.Sprintf("call:%d", card.ID),
					})
				} else {
					// A disabled button still needs a custom id; reuse the toggle id.
					buttons = append(buttons, discordgo.Button{
						Label:    "FREE",
						Style:    discordgo.PrimaryButton,
						Disabled: true,
						CustomID: fmt.Sprintf("tog:%d:%d", card.ID, idx),
					})
				}
				continue
			}
			btn := discordgo.Button{
				Label:    fmt.Sprintf("%c%d", "BINGO"[col], r+1),
				CustomID: fmt.Sprintf("tog:%d:%d", card.ID, idx),
				Style:    discordgo.SecondaryButton,
			}
			if marked[idx] {
				btn.Style = discordgo.SuccessButton
			}
			buttons = append(buttons, btn)
		}
		rows = append(rows, discordgo.ActionsRow{Components: buttons})
	}
	return rows
}

// parseIDArg splits a component custom id like "tog:12:7" into its parts after
// the prefix.
func parseIDArgs(customID string) []string {
	return strings.Split(customID, ":")
}

func atoi64(s string) (int64, bool) {
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}
