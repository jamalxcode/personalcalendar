package main

import "testing"

func TestNotesText(t *testing.T) {
	cases := []struct{ in, want string }{
		// the reported case (a Google Calendar description)
		{"<p>Ali&#x27;s last day for his Dubai/UAE visa. Reminder set 10 days before.</p>",
			"Ali's last day for his Dubai/UAE visa. Reminder set 10 days before."},
		{"<p>Line one</p><p>Line two</p>", "Line one\nLine two"},
		{"First<br>Second<br/>Third", "First\nSecond\nThird"},
		{"Bring:<ul><li>passport</li><li>photos</li></ul>", "Bring:\n• passport\n• photos"},
		{`Agenda: <a href="https://example.com/doc">the doc</a>`, "Agenda: the doc (https://example.com/doc)"},
		{`<a href="https://meet.google.com/abc-defg-hij">https://meet.google.com/abc-defg-hij</a>`, "https://meet.google.com/abc-defg-hij"},
		{`Mail <a href="mailto:sara@example.com">sara@example.com</a>`, "Mail sara@example.com"},
		{"<b>Gate</b> 21&nbsp;&amp; seat 4A", "Gate 21 & seat 4A"},
		{"<p>a</p>\n\n\n<p></p><p>b</p>", "a\n\nb"},
		// plain notes stay exactly as they are
		{"Agenda:\nhttps://example.com/doc", "Agenda:\nhttps://example.com/doc"},
		{"1 < 2 and 3 > 2", "1 < 2 and 3 > 2"},
		{"Tom &amp; Jerry", "Tom & Jerry"},
		{"", ""},
	}
	for _, c := range cases {
		if got := notesText(c.in); got != c.want {
			t.Errorf("notesText(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}
