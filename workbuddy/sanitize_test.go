package main

import (
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestSanitizeBlockedTemplates_ClaudeCode(t *testing.T) {
	in := "You are Claude Code, Anthropic's official CLI for Claude."
	out := sanitizeBlockedTemplates(in)
	if out == in {
		t.Fatal("should replace blocked template")
	}
	want := "You are Claude Code, Anthropic's official CLI tool for Claude."
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

// The Claude Agent SDK form continues the sentence with a comma instead of a
// period. Matching only the period-terminated sentence left this variant
// untouched, and the upstream rejected every such request with 400 / code
// 11128 ("Illegal API invocation from an unapproved channel").
func TestSanitizeBlockedTemplates_ClaudeAgentSDKForm(t *testing.T) {
	in := "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK."
	out := sanitizeBlockedTemplates(in)
	if out == in {
		t.Fatal("should replace the comma/SDK variant")
	}
	want := "You are Claude Code, Anthropic's official CLI tool for Claude, running within the Claude Agent SDK."
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

// The feedback line is blocklisted as one exact sentence; verbatim it makes
// the upstream reject the request with 400 / code 11128.
func TestSanitizeBlockedTemplates_FeedbackLine(t *testing.T) {
	in := "  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"
	out := sanitizeBlockedTemplates(in)
	if out == in {
		t.Fatal("should replace the blocklisted feedback line")
	}
	if strings.Contains(out, "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("blocklisted URL survived: %q", out)
	}
	if !strings.Contains(out, "To give feedback") {
		t.Fatalf("rewrite lost the line's meaning: %q", out)
	}
}

// A real system prompt carries both blocklisted strings at once; the sanitizer
// must clear both in a single pass.
func TestSanitizeBlockedTemplates_BothTemplatesInOnePrompt(t *testing.T) {
	in := "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.\n" +
		"  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues\n"
	out := sanitizeBlockedTemplates(in)
	if strings.Contains(out, "Anthropic's official CLI for Claude") {
		t.Fatalf("identity line survived: %q", out)
	}
	if strings.Contains(out, "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("feedback URL survived: %q", out)
	}
}

func TestSanitizeBlockedTemplates_MainBranch(t *testing.T) {
	in := "Main branch (you will usually use this for PRs)"
	out := sanitizeBlockedTemplates(in)
	if out == in {
		t.Fatal("should replace Main branch")
	}
}

func TestSanitizeBlockedTemplates_Codex(t *testing.T) {
	in := "You are a coding agent running in the Codex CLI, a terminal-based coding assistant."
	out := sanitizeBlockedTemplates(in)
	if out == in {
		t.Fatal("should replace codex template")
	}
	want := "You are a coding agent running in the Codex CLI tool, a terminal-based coding assistant."
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestRewriteSystemInPlace_TopLevelSystemString(t *testing.T) {
	feedback := "  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"
	obj := map[string]any{
		"model":  "deepseek-v4.1-flash-sg",
		"system": feedback,
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	changed := rewriteSystemInPlace(obj)
	if !changed {
		t.Fatal("should mark changed for top-level system")
	}
	sys, ok := obj["system"].(string)
	if !ok || strings.Contains(sys, "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("top-level system not sanitized: %v", obj["system"])
	}
}

func TestRewriteSystemInPlace_TopLevelSystemArray(t *testing.T) {
	feedback := "  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"
	obj := map[string]any{
		"model": "deepseek-v4.1-flash-sg",
		"system": []any{
			map[string]any{"type": "text", "text": feedback},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	changed := rewriteSystemInPlace(obj)
	if !changed {
		t.Fatal("should mark changed for top-level system array")
	}
	parts, ok := obj["system"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("system is not []any: %v", obj["system"])
	}
	part0 := parts[0].(map[string]any)
	text := part0["text"].(string)
	if strings.Contains(text, "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("top-level system array block not sanitized: %v", text)
	}
}

func TestRewriteSystemInPlace_InstructionsAndGemini(t *testing.T) {
	feedback := "  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"
	obj := map[string]any{
		"instructions": feedback,
		"system_instruction": map[string]any{
			"parts": []any{
				map[string]any{"text": feedback},
			},
		},
	}
	changed := rewriteSystemInPlace(obj)
	if !changed {
		t.Fatal("should mark changed")
	}
	if strings.Contains(obj["instructions"].(string), "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("instructions not sanitized: %v", obj["instructions"])
	}
	si := obj["system_instruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(si, "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("system_instruction not sanitized: %v", si)
	}
}

func TestRewriteSystemInPlace_ToolCallsAndReasoning(t *testing.T) {
	feedback := "  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues"
	obj := map[string]any{
		"messages": []any{
			map[string]any{
				"role":              "assistant",
				"reasoning_content": feedback,
				"reasoning":         feedback,
				"tool_calls": []any{
					map[string]any{
						"function": map[string]any{
							"name":      "view",
							"arguments": feedback,
						},
					},
				},
			},
		},
	}
	changed := rewriteSystemInPlace(obj)
	if !changed {
		t.Fatal("should mark changed")
	}
	msg := obj["messages"].([]any)[0].(map[string]any)
	if strings.Contains(msg["reasoning_content"].(string), "https://github.com/anthropics/claude-code/issues") {
		t.Fatal("reasoning_content not sanitized")
	}
	if strings.Contains(msg["reasoning"].(string), "https://github.com/anthropics/claude-code/issues") {
		t.Fatal("reasoning not sanitized")
	}
	tc := msg["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"].(string)
	if strings.Contains(tc, "https://github.com/anthropics/claude-code/issues") {
		t.Fatal("tool_calls arguments not sanitized")
	}
}

func TestRewriteSystemForUpstream_TopLevelSystem(t *testing.T) {
	in := []byte(`{"system":"  - To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues","messages":[{"role":"user","content":"hi"}]}`)
	out := rewriteSystemForUpstream(in)
	if strings.Contains(string(out), "https://github.com/anthropics/claude-code/issues") {
		t.Fatalf("feedback survived: %s", string(out))
	}
}

func TestSanitizeBlockedTemplates_NoMatch(t *testing.T) {
	in := "Hello world"
	out := sanitizeBlockedTemplates(in)
	if out != in {
		t.Fatal("should pass through unchanged")
	}
}

func TestForceMaxThinking_Hy3Model(t *testing.T) {
	obj := map[string]any{"model": "hy3-std"}
	changed := forceMaxThinking(obj)
	if !changed {
		t.Fatal("should change hy3 model")
	}
	if obj["reasoning_effort"] != "high" {
		t.Fatal("should set high")
	}
}

func TestForceMaxThinking_NonHy3Model(t *testing.T) {
	obj := map[string]any{"model": "glm-5.2"}
	changed := forceMaxThinking(obj)
	if changed {
		t.Fatal("should not change non-hy3 model")
	}
}

func TestForceMaxThinking_AlreadyHigh(t *testing.T) {
	obj := map[string]any{"model": "hy3-std", "reasoning_effort": "high"}
	changed := forceMaxThinking(obj)
	if changed {
		t.Fatal("should not change when already high")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Fatal("short string should be unchanged")
	}
	if truncate("hello world", 5) != "hello" {
		t.Fatal("should truncate to 5 chars")
	}
	if truncate("", 5) != "" {
		t.Fatal("empty string")
	}
}

func TestSanitizeBlockedTemplates_DeepSeekHarness(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "verbatim",
			in:   "You are an AI agent powered by DeepSeek Harness. Follow the rules.",
			want: "You are an AI assistant. Follow the rules.",
		},
		{
			name: "no trailing period",
			in:   "You are an AI agent powered by DeepSeek Harness",
			want: "You are an AI assistant",
		},
		{
			name: "case drift",
			in:   "you are an AI agent powered by deepseek harness.",
			want: "You are an AI assistant.",
		},
		{
			name: "embedded in a long prompt",
			in:   "Intro line.\nYou are an AI agent powered by DeepSeek Harness.\nTools follow.",
			want: "Intro line.\nYou are an AI assistant.\nTools follow.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeBlockedTemplates(tc.in); got != tc.want {
				t.Fatalf("sanitizeBlockedTemplates(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Near-misses must survive untouched: only the full "You are an AI agent
// powered by DeepSeek Harness" sentence is blocklisted (verified by replay —
// each of these returned 200 against the live gateway).
func TestSanitizeBlockedTemplates_DeepSeekHarnessNearMisses(t *testing.T) {
	untouched := []string{
		"DeepSeek Harness",
		"powered by DeepSeek Harness",
		"You are an AI agent powered by DeepSeek.",
		"You are an AI agent powered by Acme Harness.",
		"AI agent powered by DeepSeek Harness.",
	}
	for _, in := range untouched {
		if got := sanitizeBlockedTemplates(in); got != in {
			t.Fatalf("sanitizeBlockedTemplates(%q) = %q, want it untouched", in, got)
		}
	}
}

// The rewrite must survive the whole request pipeline (system message inside a
// chat-completions body), not just the string helper.
func TestRewriteSystemForUpstream_DeepSeekHarness(t *testing.T) {
	in := []byte(`{"messages":[{"role":"system","content":"You are an AI agent powered by DeepSeek Harness. Be brief."},{"role":"user","content":"hi"}]}`)
	out := rewriteSystemForUpstream(in)
	if strings.Contains(string(out), "powered by DeepSeek Harness") {
		t.Fatalf("harness intro survived: %s", string(out))
	}
	if !strings.Contains(string(out), "You are an AI assistant. Be brief.") {
		t.Fatalf("rewrite lost the surrounding prompt: %s", string(out))
	}
}

func TestForceMaxThinking_PreservesNone(t *testing.T) {
	obj := map[string]any{"model": "hy3-std", "reasoning_effort": "none"}
	if forceMaxThinking(obj) {
		t.Fatal("should preserve explicit none")
	}
	if obj["reasoning_effort"] != "none" {
		t.Fatalf("got %v, want none", obj["reasoning_effort"])
	}
}

func TestNormalizeThinkingInPlace(t *testing.T) {
	// 1. Adaptive + output_config.effort via original request fallback
	obj := map[string]any{"model": "deepseek-v4-pro"}
	orig := []byte(`{"model":"cn/deepseek-v4-pro","thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`)
	if !normalizeThinkingInPlace(obj, orig) {
		t.Fatal("expected change from original adaptive effort")
	}
	if obj["reasoning_effort"] != "low" {
		t.Fatalf("got %v, want low", obj["reasoning_effort"])
	}

	// 2. Disabled strips thinking/output_config and sets none
	obj2 := map[string]any{
		"model":         "deepseek-v4.1-flash",
		"thinking":      map[string]any{"type": "disabled"},
		"output_config": map[string]any{"effort": "high"},
	}
	if !normalizeThinkingInPlace(obj2, nil) {
		t.Fatal("expected change for disabled thinking")
	}
	if obj2["reasoning_effort"] != "none" {
		t.Fatalf("got %v, want none", obj2["reasoning_effort"])
	}
	if _, exists := obj2["thinking"]; exists {
		t.Fatal("thinking field should be deleted")
	}
	if _, exists := obj2["output_config"]; exists {
		t.Fatal("output_config field should be deleted")
	}

	// 3. Enabled with budget_tokens
	obj3 := map[string]any{
		"model":    "kimi-k3.1-pro",
		"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(8000)},
	}
	normalizeThinkingInPlace(obj3, nil)
	if obj3["reasoning_effort"] != "medium" {
		t.Fatalf("got %v, want medium", obj3["reasoning_effort"])
	}

	// 4. Hy3 keeps high for non-none, preserves none
	obj4 := map[string]any{"model": "hy3", "reasoning_effort": "low"}
	normalizeThinkingInPlace(obj4, nil)
	if obj4["reasoning_effort"] != "high" {
		t.Fatalf("hy3 got %v, want high", obj4["reasoning_effort"])
	}
	obj5 := map[string]any{"model": "hy3", "reasoning_effort": "none"}
	normalizeThinkingInPlace(obj5, nil)
	if obj5["reasoning_effort"] != "none" {
		t.Fatalf("hy3 disabled got %v, want none", obj5["reasoning_effort"])
	}
}

func TestApplyThinkingSupport(t *testing.T) {
	in := []pluginapi.ModelInfo{
		{ID: "cn/deepseek-v4-pro"},
		{ID: "deepseek-v4.1-flash"},
		{ID: "cn/kimi-k3.1-pro"},
		{ID: "cn/hy4-preview"},
		{ID: "cn/glm-5.3"},
		{ID: "cn/mimo-v2.6-pro"},
		{ID: "minimax-m3"},
		{ID: "cn/minimax-m3.1-flash"},
	}
	out := applyThinkingSupport(in)
	want := map[string]bool{
		"cn/deepseek-v4-pro":    true,
		"deepseek-v4.1-flash":   true,
		"cn/kimi-k3.1-pro":      true,
		"cn/hy4-preview":        true,
		"cn/glm-5.3":            false,
		"cn/mimo-v2.6-pro":      false,
		"minimax-m3":            false,
		"cn/minimax-m3.1-flash": true,
	}
	for _, m := range out {
		has := m.Thinking != nil
		if has != want[m.ID] {
			t.Fatalf("model %s Thinking=%v, want %v", m.ID, has, want[m.ID])
		}
		if has && (!m.Thinking.ZeroAllowed || len(m.Thinking.Levels) != 6) {
			t.Fatalf("model %s unexpected Thinking: %+v", m.ID, m.Thinking)
		}
	}
}
