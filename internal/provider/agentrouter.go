package provider

import (
	"net/http"
	"strings"
)

// AgentRouterUA is the Claude Code release magpie says it is to AgentRouter
// (agentrouter.org), which serves coding CLIs alone: its model list and its
// APIs answer anything else 401 "unauthorized client detected". Asked on
// 2026-10-15, it served claude-cli/2.1.161 (external, cli) and 2.1.296
// (external, sdk-cli), codex_cli_rs, opencode, Kilo-Code and Cline, and
// refused a bare "claude-cli/9.9.9", "magpie/…", "curl/…", "OpenAI/Python"
// and GeminiCLI. magpie's own requests (a list, a test, a turn an agent
// whose client it isn't started) are none of those, so they carry Claude
// Code's.
const AgentRouterUA = "claude-cli/2.1.161 (external, cli)"

// agentRouterHeaders are the headers Claude Code tells itself by, which is
// all AgentRouter's gate asks for. The request's own — anthropic-version,
// anthropic-beta — stay the request's.
var agentRouterHeaders = map[string]string{
	"User-Agent": AgentRouterUA,
	"x-app":      "cli",
}

// AgentRouterClient makes a request to AgentRouter carry Claude Code's
// User-Agent and x-app.
func AgentRouterClient(h http.Header) {
	for k, v := range agentRouterHeaders {
		h.Set(k, v)
	}
}

// IsAgentRouter reports whether the provider is AgentRouter: made from its
// preset, or at its host.
func (p Provider) IsAgentRouter() bool {
	if p.Preset == "agentrouter" {
		return true
	}
	h := p.Host()
	return h == "agentrouter.org" || strings.HasSuffix(h, ".agentrouter.org")
}
