package discordgo

import (
	"encoding/json"
	"testing"
)

func TestMessagePartialComponentsPreserveOrReplaceCache(t *testing.T) {
	for _, test := range []struct {
		name, payload, label string
		present              bool
		count                int
	}{
		{"omitted", `{"id":"m","channel_id":"c","attachments":[]}`, "old button", false, 1},
		{"explicit-empty", `{"id":"m","channel_id":"c","components":[]}`, "", true, 0},
		{"replacement", `{"id":"m","channel_id":"c","components":[{"type":2,"style":1,"label":"new button","custom_id":"button"}]}`, "new button", true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewState()
			state.MaxMessageCount = 5
			if err := state.GuildAdd(&Guild{ID: "g"}); err != nil {
				t.Fatal(err)
			}
			if err := state.ChannelAdd(&Channel{ID: "c", GuildID: "g"}); err != nil {
				t.Fatal(err)
			}
			old := &Message{ID: "m", ChannelID: "c", Components: []MessageComponent{&Button{Label: "old button"}}}
			if err := state.MessageAdd(old); err != nil {
				t.Fatal(err)
			}
			var update MessageUpdate
			if err := json.Unmarshal([]byte(test.payload), &update); err != nil {
				t.Fatal(err)
			}
			if (update.Components != nil) != test.present {
				t.Fatal("message decoder erased component presence")
			}
			if err := state.MessageAdd(update.Message); err != nil {
				t.Fatal(err)
			}
			cached, err := state.Message("c", "m")
			if err != nil {
				t.Fatal(err)
			}
			if len(cached.Components) != test.count {
				t.Fatalf("cached component count = %d, want %d", len(cached.Components), test.count)
			}
			if test.count != 0 && cached.Components[0].(*Button).Label != test.label {
				t.Fatal("partial component update changed the wrong cache value")
			}
			if test.present && test.count == 0 && cached.Components == nil {
				t.Fatal("explicit empty component set became omitted")
			}
		})
	}
}
