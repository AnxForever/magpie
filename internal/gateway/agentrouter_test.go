package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A relay that serves Claude Code clients alone gets Claude Code's headers
// on every request magpie makes to it, not only on the ones Claude Code
// asked for: agentrouter.org answers a model list, a test or a turn another
// agent started with 401 "unauthorized client detected", which its own gate
// says for any User-Agent but "claude-cli/<version> (<…>, cli)" (#179's
// class, provider.AgentRouterClient).
func TestAgentRouterSpeaksAsClaudeCode(t *testing.T) {
	f := &fake{t: t, ctype: "application/json",
		reply: `{"id":"c","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`}
	f.refuse = func([]byte) (int, string) {
		ua := f.head.Get("User-Agent")
		if !strings.HasPrefix(ua, "claude-cli/") || !strings.Contains(ua, "(") || f.head.Get("x-app") != "cli" {
			return http.StatusUnauthorized, `{"error":{"message":"unauthorized client detected"}}`
		}
		return 0, ""
	}
	setup(t, provider.Chat, f)
	p, _ := provider.Find("fake")
	p.Preset = "agentrouter"
	if err := provider.Save(*p); err != nil {
		t.Fatal(err)
	}

	// an agent that isn't Claude Code: magpie sends the relay its own
	// headers, and the relay serves the turn
	rec := httptest.NewRecorder()
	body := `{"model":"fake/m1","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("User-Agent", "opencode/1.0")
	New().Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if ua := f.head.Get("User-Agent"); ua != provider.AgentRouterUA {
		t.Errorf("User-Agent %q, want %q", ua, provider.AgentRouterUA)
	}
	if app := f.head.Get("x-app"); app != "cli" {
		t.Errorf("x-app %q", app)
	}
	if !bytes.Contains(f.got, []byte(`"model":"m1"`)) {
		t.Errorf("body %s", f.got)
	}
}
