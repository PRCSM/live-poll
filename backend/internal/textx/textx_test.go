package textx

import "testing"

func TestClean(t *testing.T) {
	cases := map[string]string{
		"  hello   world ":   "hello world",
		"tab\there\nnewline": "tab here newline",
		"zero​width":         "zerowidth",
		"rtl‮override":       "rtloverride",
		"family 👨‍👩‍👧":       "family 👨‍👩‍👧",
		"café":              "café",
		" ​​ ":               "",
		"emoji 🎉 stays":      "emoji 🎉 stays",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}
