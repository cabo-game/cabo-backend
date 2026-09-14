package actions

import "encoding/json"

// DrawCardAction lets the current-turn player draw the top card of the
// draw pile. The drawn card is held pending (game.Player.DrawnCard) — it
// does not change the player's hand size; resolving it (swap or discard)
// is a separate, not-yet-built action.
//
// Drawing does not advance the turn: the acting player still has to
// resolve the drawn card (swap it in, use a power, or discard it — none
// of that is built yet) before their turn actually ends. AdvanceTurn
// belongs to whichever action resolves the pending draw, not to the draw
// itself.
//
// Only the drawer is told which card they drew — Cabo hides it from every
// other player. Every other seated player is still notified that a draw
// happened (so they know a turn is in progress), but their payload
// carries only the drawer's ID, never the card.
//
// Exported (not the lowercase drawCardAction the LLD first sketched) so
// package gameplay's action registry can reference it by name (see
// GAMEPLAY_LLD_DRAFT.md's package-boundary revision) — the registry key
// is derived from DrawCardAction{}.Name(), not duplicated as a separate
// string literal.
type DrawCardAction struct{}

func (DrawCardAction) Name() string { return "draw_card" }

func (DrawCardAction) RequiresCurrentTurn() bool { return true }

func (DrawCardAction) Handle(ctx ActionContext) (Effect, error) {
	drawn, err := ctx.Room.DrawTopCard(ctx.ActingPlayerID)
	if err != nil {
		return Effect{}, err
	}

	drawerPayload, err := json.Marshal(struct {
		PlayerID string `json:"player_id"`
		Card     any    `json:"card"`
	}{PlayerID: ctx.ActingPlayerID, Card: drawn})
	if err != nil {
		return Effect{}, err
	}

	othersPayload, err := json.Marshal(struct {
		PlayerID string `json:"player_id"`
	}{PlayerID: ctx.ActingPlayerID})
	if err != nil {
		return Effect{}, err
	}

	recipients := []Recipient{
		{PlayerID: ctx.ActingPlayerID, View: PlayerView{Event: "you_drew", Payload: drawerPayload}},
	}
	for _, id := range ctx.Room.SeatedPlayerIDs() {
		if id == ctx.ActingPlayerID {
			continue
		}
		recipients = append(recipients, Recipient{
			PlayerID: id,
			View:     PlayerView{Event: "player_drew", Payload: othersPayload},
		})
	}

	return Effect{Recipients: recipients}, nil
}
