package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---- AI diagnostics: why the AI isn't working, in plain words ----

// aiError is a failed AI request, kept structured so it can be explained.
type aiError struct {
	Provider string // OpenRouter | OpenAI
	Model    string
	Status   int    // HTTP status, 0 for network trouble
	Msg      string // what the provider said
	Network  bool   // couldn't reach it at all
	Timeout  bool
}

func (e *aiError) Error() string {
	switch {
	case e.Timeout:
		return e.Provider + " took too long to answer"
	case e.Network:
		return e.Provider + " unreachable: " + e.Msg
	case e.Status > 0:
		return fmt.Sprintf("%s (%d): %s", e.Provider, e.Status, e.Msg)
	}
	return e.Provider + ": " + e.Msg
}

// explainAI turns an AI failure into a short reason and what to do about it.
func explainAI(err error, model string) (reason, fix string) {
	var ae *aiError
	var both *bothFailed
	if errors.As(err, &both) {
		r1, f1 := explainAI(both.Main, both.MainModel)
		r2, f2 := explainAI(both.Backup, both.BackupModel)
		fix := "Main AI: " + f1
		if f2 != f1 {
			fix += " Backup AI: " + f2
		}
		return "Neither AI worked — main: " + r1 + "; backup: " + r2, fix
	}
	if errors.Is(err, errNoKey) {
		return "No AI key yet", "Paste an OpenRouter key (sk-or-…) in Settings → AI. Get one at openrouter.ai/keys."
	}
	if !errors.As(err, &ae) {
		if errors.Is(err, context.DeadlineExceeded) {
			return "The AI took too long to answer", "Try again — long pasted text takes longer. If it keeps happening, try a faster model."
		}
		return "The AI's answer couldn't be used (" + err.Error() + ")", "Try again, or try another model in Settings → AI."
	}
	p := ae.Provider
	low := strings.ToLower(ae.Msg)
	switch {
	case ae.Timeout:
		return p + " took too long to answer", "Try again — long pasted text takes longer. If it keeps happening, try a faster model (e.g. openai/gpt-4o-mini)."
	case ae.Network:
		return "Couldn't reach " + p, "Check the internet connection, and that no VPN, firewall or content filter blocks " + hostOf(p) + "."
	case ae.Status == 401:
		return p + " doesn't accept this key (401)", "The key is wrong, was deleted or has expired. Make a new one at " + keysURL(p) + " and paste it here."
	case ae.Status == 402:
		return p + " says your credits have run out (402)", "Add credits at " + creditsURL(p) + ". Smart Cal uses very little — each request costs a fraction of a cent with the default model."
	case ae.Status == 403:
		return p + " refused the request (403): " + ae.Msg, "Often a safety filter or a model not offered in your region. Try again with different wording, or another model."
	case ae.Status == 404:
		return "The model “" + model + "” isn't available on " + p + " (404)", "Pick another model in Settings → AI — e.g. openai/gpt-4o-mini."
	case ae.Status == 408:
		return p + " timed out (408)", "Try again in a moment."
	case ae.Status == 429:
		return p + " says too many requests (429)", "Wait a minute and try again. On a free key the daily limit may be used up — adding credits lifts it."
	case ae.Status == 400 && (strings.Contains(low, "response_format") || strings.Contains(low, "json_schema") || strings.Contains(low, "structured")):
		return "The model “" + model + "” can't give the structured answers Smart Cal needs (400)", "Pick a model that supports structured outputs in Settings → AI — e.g. openai/gpt-4o-mini."
	case ae.Status == 400 && (strings.Contains(low, "context") || strings.Contains(low, "too long") || strings.Contains(low, "maximum")):
		return "The text is too long for the model “" + model + "” (400)", "Paste a shorter part, or choose a model with a larger context."
	case ae.Status == 400:
		return p + " rejected the request (400): " + ae.Msg, "Try again; if it repeats, try another model in Settings → AI."
	case ae.Status >= 500:
		return fmt.Sprintf("%s or the model's provider is having trouble (%d)", p, ae.Status), "It's on their side — try again in a few minutes."
	case ae.Status > 0:
		return fmt.Sprintf("%s returned an error (%d): %s", p, ae.Status, ae.Msg), "Try again; if it repeats, use Test AI in Settings → AI."
	}
	return p + ": " + ae.Msg, "Try again, or use Test AI in Settings → AI."
}

func hostOf(provider string) string {
	if provider == "OpenAI" {
		return "api.openai.com"
	}
	return "openrouter.ai"
}
func keysURL(provider string) string {
	if provider == "OpenAI" {
		return "platform.openai.com/api-keys"
	}
	return "openrouter.ai/keys"
}
func creditsURL(provider string) string {
	if provider == "OpenAI" {
		return "platform.openai.com/settings/organization/billing"
	}
	return "openrouter.ai/credits"
}

// the last AI request's outcome, for Settings → AI ("Last problem: …")
var (
	aiLastMu  sync.Mutex
	aiLastAt  time.Time
	aiLastErr string // "" = it worked
	aiLastFix string
)

func recordAI(err error, model string) {
	aiLastMu.Lock()
	defer aiLastMu.Unlock()
	aiLastAt = time.Now()
	if err == nil {
		aiLastErr, aiLastFix = "", ""
		return
	}
	aiLastErr, aiLastFix = explainAI(err, model)
}

// AIStatus is what Settings → AI shows about the last request.
type AIStatus struct {
	At     time.Time `json:"at"`
	OK     bool      `json:"ok"`
	Reason string    `json:"reason"`
	Fix    string    `json:"fix"`
}

func lastAIStatus() AIStatus {
	aiLastMu.Lock()
	defer aiLastMu.Unlock()
	return AIStatus{At: aiLastAt, OK: aiLastErr == "" && !aiLastAt.IsZero(), Reason: aiLastErr, Fix: aiLastFix}
}

// failureText: the reason and the fix in one sentence, plus what OpenRouter says about the account when the
// problem is credits, limits or the key (it's only looked up after a failure, so it costs nothing normally).
func failureText(ctx context.Context, err error, cfg *Config) string {
	why, fix := explainAI(err, cfg.OpenAIModel)
	text := why + ". " + fix
	var ae *aiError
	var both *bothFailed
	if errors.As(err, &both) {
		ae = nil
		_ = errors.As(both.Main, &ae)
	} else {
		_ = errors.As(err, &ae)
	}
	if ae != nil && ae.Provider == "OpenRouter" && (ae.Status == 401 || ae.Status == 402 || ae.Status == 403 || ae.Status == 429) {
		if acct := openRouterAccount(ctx, cfg.OpenAIKey); acct != "" {
			text += " OpenRouter shows: " + acct + "."
		}
	}
	return text
}

// openRouterAccount: "$4.12 of $10 credits left · free-tier limits" — whatever OpenRouter reports ("" if nothing).
func openRouterAccount(ctx context.Context, key string) string {
	get := func(path string, v any) int {
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1"+path, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			_ = json.NewDecoder(resp.Body).Decode(v)
		}
		return resp.StatusCode
	}
	var parts []string
	var credits struct {
		Data struct {
			Total float64 `json:"total_credits"`
			Used  float64 `json:"total_usage"`
		} `json:"data"`
	}
	if get("/credits", &credits) == 200 && (credits.Data.Total > 0 || credits.Data.Used > 0) {
		left := credits.Data.Total - credits.Data.Used
		parts = append(parts, fmt.Sprintf("$%.2f of $%.2f credits left", left, credits.Data.Total))
		if left <= 0.01 {
			parts = append(parts, "no credits left — add some at openrouter.ai/credits")
		}
	}
	var k struct {
		Data struct {
			Usage          float64  `json:"usage"`
			Limit          *float64 `json:"limit"`
			LimitRemaining *float64 `json:"limit_remaining"`
			FreeTier       bool     `json:"is_free_tier"`
		} `json:"data"`
	}
	if get("/key", &k) == 200 {
		if k.Data.Limit != nil && k.Data.LimitRemaining != nil {
			parts = append(parts, fmt.Sprintf("this key's limit: $%.2f of $%.2f left", *k.Data.LimitRemaining, *k.Data.Limit))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("used $%.2f with this key", k.Data.Usage))
		}
		if k.Data.FreeTier {
			parts = append(parts, "free tier (no credits bought yet — paid models like GPT-4o mini need credits)")
		}
	}
	return strings.Join(parts, " · ")
}

// ---- which company a key belongs to, and checking it with that company ----

// keyKind names the service from the key's prefix ("" when it's empty). supported is false for
// keys Smart Cal can't use; the prefix is only a convention, so a saved key is also checked with
// the service itself (checkKey).
func keyKind(key string) (service string, supported bool) {
	k := strings.TrimSpace(key)
	switch {
	case k == "":
		return "", true
	case strings.HasPrefix(k, "sk-or-"):
		return "OpenRouter", true
	case strings.HasPrefix(k, "sk-ant-"):
		return "Anthropic (Claude)", false
	case strings.HasPrefix(k, "AIza"):
		return "Google (Gemini)", false
	case strings.HasPrefix(k, "gsk_"):
		return "Groq", false
	case strings.HasPrefix(k, "xai-"):
		return "xAI (Grok)", false
	case strings.HasPrefix(k, "sk-"):
		return "OpenAI", true
	}
	return "an unknown service", false
}

// unsupportedKeyErr explains why a key can't be saved ("" when it can).
func unsupportedKeyErr(key string) string {
	svc, ok := keyKind(key)
	if ok {
		return ""
	}
	if svc == "an unknown service" {
		return "That doesn't look like an OpenRouter key (sk-or-…) or an OpenAI key (sk-…) — copy the whole key again."
	}
	return "That's a " + svc + " key — Smart Cal works with OpenRouter keys (sk-or-…) and OpenAI keys (sk-…). Tip: an OpenRouter key also gives you Claude, Gemini, Grok and others."
}

// KeyCheck is what the service said about a key when it was saved.
type KeyCheck struct {
	At   time.Time `json:"at"`
	OK   bool      `json:"ok"`
	Text string    `json:"text"` // "Checked with OpenAI ✓" or the reason it was refused
}

// checkKey asks the key's own service whether it accepts the key (OpenRouter: /key, OpenAI: /models).
func checkKey(ctx context.Context, key string) KeyCheck {
	svc, ok := keyKind(key)
	res := KeyCheck{At: time.Now()}
	if !ok || svc == "" {
		res.Text = unsupportedKeyErr(key)
		return res
	}
	url := "https://api.openai.com/v1/models"
	if svc == "OpenRouter" {
		url = "https://openrouter.ai/api/v1/key"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		res.Text = "Couldn't reach " + svc + " to check the key — it will be tried when you use the AI."
		return res
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode == 200:
		res.OK, res.Text = true, svc+" accepted the key ✓"
		if svc == "OpenRouter" {
			if acct := openRouterAccount(ctx, key); acct != "" {
				res.Text += " · " + acct
				if strings.Contains(acct, "no credits left") {
					// the key is fine but the AI can't answer until there's credit — say so in red, not green
					res.OK = false
					res.Text = "OpenRouter accepted the key, but the account has no credits left, so the AI can't answer yet — add some at openrouter.ai/credits. (" + acct + ")"
				}
			}
		}
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		res.Text = fmt.Sprintf("%s doesn't accept this key (%d) — it's wrong, deleted or expired. Make a new one at %s.", svc, resp.StatusCode, keysURL(svc))
	default:
		res.Text = fmt.Sprintf("%s answered %d when checking the key — it will be tried when you use the AI.", svc, resp.StatusCode)
	}
	return res
}
