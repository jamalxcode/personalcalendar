package main

import (
	"html"
	"regexp"
	"strings"
)

// Google Calendar stores event descriptions as HTML ("<p>Ali&#x27;s visa…</p>", "<br>", links,
// lists). EventKit hands that over as-is, so turn it into plain text before the app shows,
// searches or edits it. Plain notes (from iCloud, or typed in Smart Cal) pass through unchanged.

var (
	reHTMLTag    = regexp.MustCompile(`(?i)</?(p|br|div|span|a|b|i|u|em|strong|ul|ol|li|h[1-6]|font|blockquote|pre|code|table|tr|td|th|html|body)\b[^>]*>`)
	reHTMLEntity = regexp.MustCompile(`&(#[0-9]+|#x[0-9a-fA-F]+|amp|lt|gt|quot|apos|nbsp);`)
	reBreak      = regexp.MustCompile(`(?i)<br\s*/?>`)
	reBlockEnd   = regexp.MustCompile(`(?i)</(p|div|h[1-6]|blockquote|pre|tr|ul|ol)>`)
	reListItem   = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	reLink       = regexp.MustCompile(`(?is)<a\b[^>]*?href\s*=\s*["']([^"']*)["'][^>]*>(.*?)</a>`)
	reAnyTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlankLines = regexp.MustCompile(`\n[ \t]*\n(?:[ \t]*\n)+`)
	reTrailingWS = regexp.MustCompile(`[ \t]+\n`)
)

// notesText returns the notes as plain text.
func notesText(s string) string {
	if !reHTMLTag.MatchString(s) {
		if reHTMLEntity.MatchString(s) {
			return html.UnescapeString(s)
		}
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ") // in HTML, line breaks in the source are just spaces
	// links: keep the address when the visible text is something else ("agenda" → "agenda (https://…)")
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		p := reLink.FindStringSubmatch(m)
		href := html.UnescapeString(p[1])
		text := strings.TrimSpace(html.UnescapeString(reAnyTag.ReplaceAllString(p[2], "")))
		switch {
		case text == "" || text == href || strings.TrimPrefix(href, "mailto:") == text:
			if text == "" {
				return href
			}
			return text
		case strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://"):
			return text + " (" + href + ")"
		}
		return text
	})
	s = reBreak.ReplaceAllString(s, "\n")
	s = reListItem.ReplaceAllString(s, "\n• ")
	s = reBlockEnd.ReplaceAllString(s, "\n")
	s = reAnyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	s = reTrailingWS.ReplaceAllString(s, "\n")
	s = reBlankLines.ReplaceAllString(s, "\n\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
