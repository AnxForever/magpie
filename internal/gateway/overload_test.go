package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// What the ChatGPT backend sent Aliceonly's Codex on plus-B (#1418): a 200
// and the frames before a reply, nothing said, and 36s on the response
// failed with server_is_overloaded.
var overloadedLead = []string{
	`event: response.created` + "\n" + `data: {"type":"response.created","response":{"id":"r0","status":"in_progress"}}`,
	`event: response.in_progress` + "\n" + `data: {"type":"response.in_progress","response":{"id":"r0","status":"in_progress"}}`,
	`event: codex.rate_limits` + "\n" + `data: {"type":"codex.rate_limits","plan_type":"plus","rate_limits":{"allowed":true}}`,
}

const overloadedFailed = `event: response.failed` + "\n" + `data: {"type":"response.failed","response":{"id":"r0","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`

// A stream with nothing but its frames, held past 15s while its agent is
// kept alive with comments, stays held: its server_is_overloaded after
// that is a failure the group's next member answers, not the agent's
// error (#1418). Before, the frame that came after 15s let the stream
// through, and the failure 21s later reached Codex as the turn's end with
// sub2api, next in the group, never asked. A Gemini stream, whose agent
// takes no comment, is let through at 15s as before.
func TestLeadOnlyStreamHeldWhileKeptAlive(t *testing.T) {
	held := func(proto provider.Protocol) (*holdWriter, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		h := newHoldWriter(rec, true)
		h.alive, h.streams = &keptAlive{proto: proto}, true
		h.Header().Set("Content-Type", "text/event-stream")
		h.WriteHeader(200)
		io.WriteString(h, sse(overloadedLead[:2]...))
		h.since = time.Now().Add(-(holdLongest + 5*time.Second))
		io.WriteString(h, sse(overloadedLead[2]))
		io.WriteString(h, ": ping\n\n")
		return h, rec
	}
	h, rec := held(provider.Responses)
	if h.passing || strings.Contains(rec.Body.String(), "response.created") {
		t.Fatalf("let through after %s with nothing said: %q", holdLongest, rec.Body.String())
	}
	io.WriteString(h, sse(overloadedFailed))
	if !h.failed() || h.code() != 529 || !strings.Contains(h.failMsg, "server_is_overloaded") || strings.Contains(rec.Body.String(), "server_is_overloaded") {
		t.Fatalf("failed %v (%d %q); sent %q", h.failed(), h.code(), h.failMsg, rec.Body.String())
	}

	// held so long, it is let through as before
	h, rec = held(provider.Responses)
	h.since = time.Now().Add(-(holdLead + time.Second))
	io.WriteString(h, ": ping\n\n")
	if !h.passing || !strings.Contains(rec.Body.String(), "response.created") {
		t.Fatalf("held past %s: %q", holdLead, rec.Body.String())
	}

	h, rec = held(provider.Gemini)
	if !h.passing || !strings.Contains(rec.Body.String(), "response.created") {
		t.Fatalf("a stream whose agent takes no comment held past %s", holdLongest)
	}
}

// Every member of a group overloaded at once (#1418: plus-B, then
// sub2api, each server_is_overloaded) isn't the agent's error while the
// group's patience lasts: after a pause each is asked again, and the one
// back by then answers. Before, the last one's 529 ended Codex's turn.
// Patience off, the agent gets it as before; a Retry-After past the
// patience isn't waited for.
func TestGroupWaitsOutEveryMemberOverloaded(t *testing.T) {
	overloaded := reply{529, "", `{"error":{"message":"Our servers are currently overloaded. Please try again later.","code":"server_is_overloaded"}}`}
	group := func(patience int) (*scripted, *scripted) {
		fresh(t)
		a := &scripted{replies: []reply{overloaded, {200, "", chatOK}}}
		b := &scripted{replies: []reply{overloaded}}
		scriptedOn(t, "a", provider.Chat, a)
		scriptedOn(t, "b", provider.Chat, b)
		if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m"}, Routing: provider.Ordered, Patience: patience}); err != nil {
			t.Fatal(err)
		}
		return a, b
	}
	ask := `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`

	a, b := group(0)
	s := New()
	code, body := postAs(t, s, "", ask)
	if code != 200 || !strings.Contains(body, "hello") || a.n != 2 {
		t.Fatalf("%d %s (a %d, b %d)", code, body, a.n, b.n)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	var replanned *Try
	for i := range r.Tries {
		if r.Tries[i].Replan > 0 {
			replanned = &r.Tries[i]
		}
	}
	if replanned == nil || replanned.Replan != 2 || replanned.Again == 0 || replanned.Patience != provider.PatienceDefault {
		t.Fatalf("the trace doesn't say the members were asked again: %+v", r.Tries)
	}
	if last := r.Tries[len(r.Tries)-1]; last.Status != 200 {
		t.Fatalf("tries: %+v", r.Tries)
	}

	a, b = group(provider.PatienceOff)
	if code, _ := postAs(t, New(), "", ask); code != 529 || a.n != 1 {
		t.Fatalf("patience off: %d (a %d, b %d)", code, a.n, b.n)
	}

	a, b = group(5)
	limited := reply{429, "", `{"error":{"message":"Rate limit reached, please try again shortly"}}`}
	a.limited, b.limited = http.Header{"Retry-After": {"9"}}, http.Header{"Retry-After": {"9"}}
	a.replies[0], b.replies[0] = limited, limited
	began := time.Now()
	if code, _ := postAs(t, New(), "", ask); code != 429 || a.n != 1 || time.Since(began) > 3*time.Second {
		t.Fatalf("a Retry-After past the patience: %d after %s (a %d, b %d)", code, time.Since(began), a.n, b.n)
	}
}

// What Codex was sent (#1418): plus-B's 200 and frames, then
// server_is_overloaded, and sub2api the same. With patience, the agent's
// stream is the reply of the one asked again, and nothing of the failures.
func TestCodexStreamWaitsOutOverloadedGroup(t *testing.T) {
	fresh(t)
	failed := reply{200, "text/event-stream", sse(append(slices.Clone(overloadedLead), overloadedFailed)...)}
	done := reply{200, "text/event-stream", sse(
		`event: response.output_text.delta`+"\n"+`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`event: response.completed`+"\n"+`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}`)}
	a := &scripted{replies: []reply{failed, done}}
	b := &scripted{replies: []reply{failed}}
	responsesOn(t, "a", a)
	responsesOn(t, "b", b)
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	code, body := sendTo(New(), "/v1/responses", `{"model":"group/g","stream":true,"input":"hi"}`)
	if code != 200 || !strings.Contains(body, "hello") || strings.Contains(body, "server_is_overloaded") || a.n != 2 {
		t.Fatalf("%d %q (a %d, b %d)", code, body, a.n, b.n)
	}
}
