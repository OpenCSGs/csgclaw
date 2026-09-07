package agents

import "context"

var locateCodexCLI = func() (string, error) { return "/test/codex", nil }

func (f fakeAgentRuntime) CheckAvailability(context.Context) error {
	if f.kind == RuntimeKindCodex {
		_, err := locateCodexCLI()
		return err
	}
	return nil
}
