package execution

import (
	"csgclaw/internal/channel/feishu/presentation"
	state "csgclaw/internal/channel/feishu/state"
	"testing"
)

func TestReplyIntentsMarkOnlyFinalSnapshots(t *testing.T) {
	r := &Runner{state: state.NewStore()}
	message := runnerMessage("event", "turn", "conversation", "hello")
	rendered := presentation.Rendered{Cards: []map[string]any{presentation.Card("page1"), presentation.Card("page2")}}
	for _, final := range []bool{false, true} {
		intents := r.replyIntents(message, 1, final, rendered)
		if len(intents) != 2 {
			t.Fatal("missing pages")
		}
		for _, intent := range intents {
			if intent.FinalReply != final {
				t.Fatal("incorrect replacement eligibility")
			}
		}
	}
}
