package main

import (
	"strings"
	"testing"
)

func TestExplainAI(t *testing.T) {
	cases := []struct {
		err  error
		want string // must appear in the reason
		fix  string // must appear in the fix
	}{
		{&aiError{Provider: "OpenRouter", Status: 402, Msg: "Insufficient credits"}, "credits have run out", "openrouter.ai/credits"},
		{&aiError{Provider: "OpenRouter", Status: 401, Msg: "No auth credentials found"}, "doesn't accept this key", "openrouter.ai/keys"},
		{&aiError{Provider: "OpenRouter", Status: 404, Msg: "No endpoints found"}, "isn't available", "another model"},
		{&aiError{Provider: "OpenRouter", Status: 429, Msg: "Rate limit exceeded"}, "too many requests", "Wait a minute"},
		{&aiError{Provider: "OpenRouter", Status: 400, Msg: "response_format json_schema is not supported"}, "structured answers", "structured outputs"},
		{&aiError{Provider: "OpenRouter", Status: 503, Msg: "upstream error"}, "having trouble", "few minutes"},
		{&aiError{Provider: "OpenRouter", Network: true, Msg: "dial tcp: no such host"}, "Couldn't reach OpenRouter", "openrouter.ai"},
		{&aiError{Provider: "OpenAI", Timeout: true}, "took too long", "Try again"},
		{errNoKey, "No AI key", "Settings → AI"},
	}
	for _, c := range cases {
		why, fix := explainAI(c.err, "gpt-4o-mini")
		if !strings.Contains(why, c.want) || !strings.Contains(fix, c.fix) {
			t.Errorf("%v → %q / %q", c.err, why, fix)
		}
	}
	both := &bothFailed{Main: &aiError{Provider: "OpenRouter", Status: 402}, Backup: &aiError{Provider: "OpenAI", Status: 401},
		MainModel: "gpt-4o-mini", BackupModel: "gpt-4o-mini"}
	why, fix := explainAI(both, "")
	if !strings.Contains(why, "Neither AI worked") || !strings.Contains(why, "credits") || !strings.Contains(why, "doesn't accept") ||
		!strings.Contains(fix, "Main AI:") || !strings.Contains(fix, "Backup AI:") {
		t.Errorf("both failed → %q / %q", why, fix)
	}
	if !retryable(&aiError{Status: 503}) || !retryable(&aiError{Network: true}) || retryable(&aiError{Status: 402}) || retryable(&aiError{Status: 401}) {
		t.Error("retryable: wrong")
	}
}

func TestEffectiveModel(t *testing.T) {
	cases := []struct{ key, model, provider, sent, fit string }{
		{"sk-or-v1-abc", "gpt-4o-mini", "OpenRouter", "openai/gpt-4o-mini", "gpt-4o-mini"},
		{"sk-or-v1-abc", "google/gemini-2.5-flash", "OpenRouter", "google/gemini-2.5-flash", "google/gemini-2.5-flash"},
		{"sk-proj-abc", "google/gemini-2.5-flash", "OpenAI", "gpt-4o-mini", "gpt-4o-mini"}, // the reported case
		{"sk-proj-abc", "openai/gpt-4.1-mini", "OpenAI", "gpt-4.1-mini", "gpt-4.1-mini"},
		{"sk-abc", "", "OpenAI", "gpt-4o-mini", "gpt-4o-mini"},
	}
	for _, c := range cases {
		p, s := effectiveModel(c.key, c.model)
		if p != c.provider || s != c.sent || fitModel(c.key, c.model) != c.fit {
			t.Errorf("%s + %q → %s %q (fit %q), want %s %q (fit %q)", c.key, c.model, p, s, fitModel(c.key, c.model), c.provider, c.sent, c.fit)
		}
	}
}

func TestKeyKind(t *testing.T) {
	cases := []struct {
		key, svc string
		ok       bool
	}{
		{"sk-or-v1-abc", "OpenRouter", true},
		{"sk-proj-abc", "OpenAI", true},
		{"sk-svcacct-abc", "OpenAI", true},
		{"sk-abc", "OpenAI", true},
		{"sk-ant-api03-abc", "Anthropic (Claude)", false},
		{"AIzaSyAbc", "Google (Gemini)", false},
		{"gsk_abc", "Groq", false},
		{"xai-abc", "xAI (Grok)", false},
		{"hello", "an unknown service", false},
	}
	for _, c := range cases {
		svc, ok := keyKind(c.key)
		if svc != c.svc || ok != c.ok {
			t.Errorf("%q → %q %v", c.key, svc, ok)
		}
	}
	if unsupportedKeyErr("sk-ant-x") == "" || unsupportedKeyErr("sk-proj-x") != "" {
		t.Error("unsupportedKeyErr")
	}
}
