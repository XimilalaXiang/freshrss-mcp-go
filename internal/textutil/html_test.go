package textutil

import "testing"

func TestStripHTML(t *testing.T) {
	s := StripHTML(`<p>Hello <b>world</b> &amp; friends</p>`)
	if s != "Hello world & friends" {
		t.Fatalf("got %q", s)
	}
}

func TestTruncateAtWord(t *testing.T) {
	s := TruncateAtWord("one two three four five", 12)
	if len([]rune(s)) > 15 {
		t.Fatalf("too long: %q", s)
	}
}
