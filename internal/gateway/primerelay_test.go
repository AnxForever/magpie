package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// Prime Relay's keys are group-scoped, and some of its groups answer
// Anthropic's Messages with 403 "This group does not allow /v1/messages
// dispatch". wrongEndpoint doesn't recognize that wording, so a request
// sent to that API reaches the agent as a failure of the model, with no
// other API tried. The preset sets no Anthropic base for it, and Claude
// Code's turn is translated to Chat Completions, which every group serves.
func TestPrimeRelayClaudeCodeTurnGoesToChat(t *testing.T) {
	fresh(t)
	var mu sync.Mutex
	var asked []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		mu.Lock()
		asked = append(asked, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/v1/messages" {
			// the group's own refusal, verbatim
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"error":{"message":"This group does not allow /v1/messages dispatch","type":"permission_error"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sse(
			`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`,
			`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1}}}`,
		))
	}))
	defer up.Close()
	// the preset itself, with every address it names moved to the fake
	// server: an Anthropic base added to the preset would be asked, and the
	// group's 403 handed back as the model's failure
	p, err := provider.FromPreset("primerelay")
	if err != nil {
		t.Fatal(err)
	}
	p.Key, p.Models = "sk-x", []string{"claude-sonnet-5"}
	p.Website, p.KeysURL = "", ""
	p.Chat = up.URL + "/v1"
	if p.Anthropic != "" {
		p.Anthropic = up.URL
	}
	if p.Responses != "" {
		p.Responses = up.URL + "/v1"
	}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	h := New().Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"primerelay/claude-sonnet-5","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	got := append([]string(nil), asked...)
	mu.Unlock()
	if len(got) == 0 || got[0] != "/v1/chat/completions" {
		t.Fatalf("asked %v", got)
	}
	for _, path := range got {
		if path == "/v1/messages" {
			t.Fatalf("the group's Messages API was asked: %v", got)
		}
	}
}
