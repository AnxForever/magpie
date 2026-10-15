package provider

import (
	"slices"
	"testing"
)

// Prime Relay (primerelay.online) is a New API relay whose keys are
// group-scoped: a key's group serves its own models alone, a model of
// another group answers 404 "not available for this group", and some
// groups refuse Anthropic's Messages outright ("This group does not allow
// /v1/messages dispatch", 403) — a wording wrongEndpoint doesn't read as
// "ask another API", so it would reach the agent as the model's failure.
// The preset therefore sets no Anthropic endpoint: Claude Code's turns are
// translated to Chat Completions, which every group serves, and Chat
// carries the prompt cache as Messages does. A user whose key's group does
// serve Messages can still set Anthropic in the editor.
func TestPrimeRelayPreset(t *testing.T) {
	p, err := FromPreset("primerelay")
	if err != nil {
		t.Fatal(err)
	}
	if p.Chat != "https://primerelay.online/v1" || p.Responses != "https://primerelay.online/v1" || p.Anthropic != "" {
		t.Fatalf("endpoints: %q %q %q", p.Chat, p.Responses, p.Anthropic)
	}
	if p.Name != "Prime Relay" || p.Icon != "primerelay-color" || p.Website != "https://primerelay.online" || p.KeysURL != "https://primerelay.online/keys" {
		t.Fatalf("preset: %+v", p)
	}
	if pr := Preset("primerelay"); pr.Kind != KindRelay || pr.NoKey || pr.Note == "" || pr.Sponsored {
		t.Fatalf("preset def: %+v", pr)
	}
	// Claude Code's turn is translated, never sent to a Messages API the
	// group may refuse
	if !slices.Contains(p.Speaks(), Chat) || slices.Contains(p.Speaks(), Anthropic) {
		t.Fatalf("speaks %v", p.Speaks())
	}
	// an entry another app has at the relay is the preset, not a custom one
	im, why := imported("Prime Relay", "sk-x", endpoints{chat: "https://primerelay.online/v1"}, nil)
	if why != "" || im.Preset != "primerelay" || im.Icon != "primerelay-color" || im.Anthropic != "" {
		t.Fatalf("imported: %q %+v", why, im)
	}
}
