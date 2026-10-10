package gui

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// StringKe on Discord: the menu bar shows one account's use, picked in
// Settings, not the one in use. A card id "claude|*" follows the account
// the gateway goes to first, whichever that is now, and the tooltip names it.
func TestTrayInUse(t *testing.T) {
	inUse := "b@x.y"
	was := trayInUseAccount
	trayInUseAccount = func(agent string) string {
		if agent != "claude" {
			t.Errorf("asked about %q", agent)
		}
		return inUse
	}
	t.Cleanup(func() { trayInUseAccount = was })
	cards := []provider.SubscriptionQuota{
		{Provider: "claude", Name: "Claude Code", User: "a@x.y", Windows: []provider.QuotaWindow{{Name: "5-hour", Used: 10}}},
		{Provider: "claude", Name: "Claude Code", User: "B@x.y", Windows: []provider.QuotaWindow{{Name: "5-hour", Used: 70}}},
		{Provider: "zcode", Name: "ZCode", User: "z@x.y", Windows: []provider.QuotaWindow{{Name: "5 小时", Used: 5}}},
		{Provider: "deepseek", Name: "DeepSeek", Balance: "¥1.00"},
	}
	got := trayPick(cards, []string{"claude|*", "zcode|*", "deepseek", "claude|a@x.y", "gone|*"}, time.Now())
	if len(got) != 4 {
		t.Fatalf("cards: %+v", got)
	}
	if got[0].User != "B@x.y" || got[0].Name != "Claude Code · B@x.y" {
		t.Errorf("in use: %+v", got[0])
	}
	if got[1].User != "z@x.y" || got[1].Name != "ZCode · z@x.y" {
		t.Errorf("one account: %+v", got[1])
	}
	if got[2].Name != "DeepSeek" || got[3].User != "a@x.y" || got[3].Name != "Claude Code" {
		t.Errorf("named ones as they were: %+v", got[2:])
	}
	if label, tip := trayUsageText(got[0], time.Now(), false); label != "70%" || tip != "Claude Code · B@x.y\n5-hour 70% used" {
		t.Errorf("text: %q %q", label, tip)
	}
	// switched: the card follows
	inUse = "a@x.y"
	if got = trayPick(cards, []string{"claude|*"}, time.Now()); len(got) != 1 || got[0].User != "a@x.y" {
		t.Errorf("after a switch: %+v", got)
	}
	// can't tell: the first, the one in use first among them
	inUse = ""
	if got = trayPick(cards, []string{"claude|*"}, time.Now()); len(got) != 1 || got[0].User != "a@x.y" {
		t.Errorf("unknown: %+v", got)
	}
}

// okingkee on X: three Codex accounts, the menu bar on "account in use".
// A used its allowance up and the gateway went on to B, but the menu bar
// stayed on A: it asked whom the agent is signed in to. It follows the
// account that answered last, while that is lately; else, as before, the
// one InUseLogin reckons the gateway goes to first.
func TestTrayInUseFollowsWhoAnswered(t *testing.T) {
	was := trayInUseAccount
	trayInUseAccount = func(string) string { return "a@x.y" }
	t.Cleanup(func() { trayInUseAccount = was })
	now := time.Now()
	at := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	cards := []provider.SubscriptionQuota{
		{Provider: "codex", Name: "Codex", User: "a@x.y", Windows: []provider.QuotaWindow{{Name: "5-hour", Used: 100}}, LastServedAt: at(40 * time.Minute)},
		{Provider: "codex", Name: "Codex", User: "b@x.y", Windows: []provider.QuotaWindow{{Name: "5-hour", Used: 12}}, LastServedAt: at(time.Minute)},
		{Provider: "codex", Name: "Codex", User: "c@x.y", Windows: []provider.QuotaWindow{{Name: "5-hour", Used: 0}}},
	}
	got := trayPick(cards, []string{"codex|*"}, now)
	if len(got) != 1 || got[0].User != "b@x.y" || got[0].Name != "Codex · b@x.y" {
		t.Fatalf("in use: %+v", got)
	}
	if label, _ := trayUsageText(got[0], now, false); label != "12%" {
		t.Errorf("label %q", label)
	}
	// nothing answered lately: whom the gateway goes to first
	cards[0].LastServedAt, cards[1].LastServedAt = at(3*time.Hour), at(2*time.Hour)
	if got = trayPick(cards, []string{"codex|*"}, now); len(got) != 1 || got[0].User != "a@x.y" {
		t.Errorf("stale answers: %+v", got)
	}
	// one account alone is the one, whoever answered
	if got = trayPick(cards[2:], []string{"codex|*"}, now); len(got) != 1 || got[0].User != "c@x.y" {
		t.Errorf("one: %+v", got)
	}
}
