package vote

import (
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/PRCSM/live-poll/backend/internal/live"
)

func TestValidateChoice(t *testing.T) {
	single := live.Meta{OptionIDs: []string{"o1", "o2", "o3"}}
	multi := live.Meta{OptionIDs: []string{"o1", "o2", "o3"}, MultiChoice: true}
	cases := []struct {
		name string
		meta live.Meta
		ids  []string
		ok   bool
	}{
		{"single ok", single, []string{"o2"}, true},
		{"empty", single, nil, false},
		{"two on single", single, []string{"o1", "o2"}, false},
		{"foreign option", single, []string{"o9"}, false},
		{"multi ok", multi, []string{"o1", "o3"}, true},
		{"multi duplicate", multi, []string{"o1", "o1"}, false},
		{"multi too many", multi, []string{"o1", "o2", "o3", "o1"}, false},
	}
	for _, tc := range cases {
		got := validateChoice(castInput{OptionIDs: tc.ids}, tc.meta) == nil
		if got != tc.ok {
			t.Errorf("%s: ok = %v", tc.name, got)
		}
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{
		"=HYPERLINK(\"x\")": "'=HYPERLINK(\"x\")",
		"+1":                "'+1",
		"-2":                "'-2",
		"@SUM(A1)":          "'@SUM(A1)",
		"Go":                "Go",
		"":                  "",
	} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q", in, got)
		}
	}
}

func TestParseEntry(t *testing.T) {
	good := redis.XMessage{ID: "1-0", Values: map[string]any{
		"pollId": "65f000000000000000000001", "voterKey": "k", "optionIds": "o1,o2",
		"userId": "", "ipHash": "h", "at": "1732500000000",
	}}
	v, err := parseEntry(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.OptionIDs) != 2 || v.UserID != nil || v.CreatedAt.UnixMilli() != 1732500000000 {
		t.Errorf("parsed %+v", v)
	}

	bad := redis.XMessage{ID: "2-0", Values: map[string]any{"pollId": "nope", "at": "1"}}
	if _, err := parseEntry(bad); err == nil {
		t.Error("malformed entry accepted")
	}
}
