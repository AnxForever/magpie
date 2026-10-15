package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// agentrouterList is AgentRouter's /models as it answers: a Claude model its
// Anthropic and OpenAI APIs both serve, and an OpenAI model.
const agentrouterList = `{"data":[
	{"id":"claude-opus-5-5","object":"model","supported_endpoint_types":["anthropic","openai"]},
	{"id":"gpt-6-astra","object":"model","supported_endpoint_types":["openai"]}
]}`

// agentRouterServer is agentrouter.org's gate as it answers a request that
// isn't Claude Code's: 401 "unauthorized client detected", the model list
// and every API alike. A request it serves is noted in seen.
func agentRouterServer(t *testing.T, seen *[]http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if !strings.HasPrefix(ua, "claude-cli/") || !strings.Contains(ua, "(") || r.Header.Get("x-app") != "cli" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"unauthorized client detected"},"message":"UNAUTHENTICATED","success":false}`)
			return
		}
		*seen = append(*seen, r.Header)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			io.WriteString(w, agentrouterList)
		case r.URL.Path == "/v1/chat/completions":
			io.WriteString(w, `{"id":"c","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`)
		case r.URL.Path == "/v1/responses":
			io.WriteString(w, `{"id":"r","object":"response","status":"completed","output":[]}`)
		case r.URL.Path == "/v1/messages":
			io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A provider from the preset asks AgentRouter as Claude Code: its model list
// and its own endpoint test alike, which the relay answers "unauthorized
// client detected" without (agentrouter.go).
func TestAgentRouterAsksAsClaudeCode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var seen []http.Header
	srv := agentRouterServer(t, &seen)
	p, err := FromPreset("agentrouter")
	if err != nil {
		t.Fatal(err)
	}
	p.ID, p.Key = "agentrouter-test", "sk-test"
	p.Chat, p.Responses, p.Anthropic = srv.URL+"/v1", srv.URL+"/v1", srv.URL
	if !p.IsAgentRouter() {
		t.Fatal("a provider from the preset isn't AgentRouter")
	}

	ms, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("the model list: %v", err)
	}
	var ids []string
	for _, m := range ms {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "claude-opus-5-5,gpt-6-astra" {
		t.Fatalf("models %v", ids)
	}
	res := p.Test(context.Background())
	if len(res) != 3 {
		t.Fatalf("the endpoint test tried %d APIs: %+v", len(res), res)
	}
	for _, r := range res {
		if !r.OK {
			t.Errorf("%s: %+v", r.Protocol, r)
		}
	}
	for _, h := range seen {
		if h.Get("User-Agent") != AgentRouterUA || h.Get("x-app") != "cli" {
			t.Errorf("asked as %q %q", h.Get("User-Agent"), h.Get("x-app"))
		}
		if h.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("auth %q", h.Get("Authorization"))
		}
	}
}

// IsAgentRouter knows the preset and the host, and nobody else.
func TestAgentRouterIdentity(t *testing.T) {
	for _, c := range []struct {
		p    Provider
		want bool
	}{
		{Provider{Preset: "agentrouter"}, true},
		{Provider{Anthropic: "https://agentrouter.org"}, true},
		{Provider{Anthropic: "https://co.agentrouter.org/v1"}, true},
		{Provider{Chat: "https://agentrouter.org"}, true},
		{Provider{Chat: "https://agentrouter.org.evil.example/v1"}, false},
		{Provider{Preset: "anthropic", Anthropic: "https://api.anthropic.com"}, false},
	} {
		if got := c.p.IsAgentRouter(); got != c.want {
			t.Errorf("%v: %v, want %v", c.p.Preset+" "+c.p.Host(), got, c.want)
		}
	}
}

// Sign gives the request Claude Code's headers, and a header of the user's
// own of the same name still wins.
func TestAgentRouterSign(t *testing.T) {
	p := Provider{ID: "agentrouter", Preset: "agentrouter", Key: "k", Anthropic: "https://agentrouter.org"}
	req, _ := http.NewRequest("POST", "https://agentrouter.org/v1/messages", nil)
	if err := p.Sign(context.Background(), req, Anthropic, nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("User-Agent") != AgentRouterUA || req.Header.Get("x-app") != "cli" {
		t.Fatalf("signed as %q %q", req.Header.Get("User-Agent"), req.Header.Get("x-app"))
	}
	if h := req.Header.Get("x-api-key"); h != "k" {
		t.Errorf("key %q", h)
	}

	p.Headers = map[string]string{"User-Agent": "mine/1"}
	req, _ = http.NewRequest("POST", "https://agentrouter.org/v1/messages", nil)
	if err := p.Sign(context.Background(), req, Anthropic, nil); err != nil {
		t.Fatal(err)
	}
	if ua := req.Header.Get("User-Agent"); ua != "mine/1" {
		t.Errorf("the user's own User-Agent: %q", ua)
	}
	if h := p.listHeaders(); h["User-Agent"] != AgentRouterUA {
		t.Errorf("list headers %v", h)
	}
}
