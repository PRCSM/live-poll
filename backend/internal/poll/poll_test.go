package poll

import (
	"strings"
	"testing"
	"time"

	"github.com/PRCSM/live-poll/backend/internal/live"
)

var now = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func validInput() createInput {
	return createInput{
		Question: "Best language?",
		Options:  []string{"Go", "Rust"},
		ClosesAt: now.Add(time.Hour).Format(time.RFC3339),
	}
}

func TestCreateValidation(t *testing.T) {
	cases := map[string]struct {
		mutate    func(*createInput)
		badFields []string
	}{
		"valid":           {func(*createInput) {}, nil},
		"blank question":  {func(in *createInput) { in.Question = " ​\t" }, []string{"question"}},
		"long question":   {func(in *createInput) { in.Question = strings.Repeat("q", 281) }, []string{"question"}},
		"one option":      {func(in *createInput) { in.Options = []string{"Go"} }, []string{"options"}},
		"eleven options":  {func(in *createInput) { in.Options = make([]string, 11) }, []string{"options"}},
		"empty option":    {func(in *createInput) { in.Options = []string{"Go", "  "} }, []string{"options.1"}},
		"long option":     {func(in *createInput) { in.Options = []string{"Go", strings.Repeat("o", 101)} }, []string{"options.1"}},
		"case duplicate":  {func(in *createInput) { in.Options = []string{"Go", " go "} }, []string{"options.1"}},
		"bad visibility":  {func(in *createInput) { in.ResultsVisibility = "never" }, []string{"resultsVisibility"}},
		"past close":      {func(in *createInput) { in.ClosesAt = now.Add(-time.Hour).Format(time.RFC3339) }, []string{"closesAt"}},
		"too soon":        {func(in *createInput) { in.ClosesAt = now.Add(30 * time.Second).Format(time.RFC3339) }, []string{"closesAt"}},
		"too far":         {func(in *createInput) { in.ClosesAt = now.Add(31 * 24 * time.Hour).Format(time.RFC3339) }, []string{"closesAt"}},
		"not a timestamp": {func(in *createInput) { in.ClosesAt = "tomorrow" }, []string{"closesAt"}},
		"offset timezone okay": {func(in *createInput) {
			in.ClosesAt = now.Add(time.Hour).In(time.FixedZone("IST", 19800)).Format(time.RFC3339)
		}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			fields, _ := in.validate(now)
			if len(fields) != len(tc.badFields) {
				t.Fatalf("fields = %v, want %v", fields, tc.badFields)
			}
			for _, f := range tc.badFields {
				if _, ok := fields[f]; !ok {
					t.Errorf("missing error for %s: %v", f, fields)
				}
			}
		})
	}
}

func TestValidationCleansAndDefaults(t *testing.T) {
	in := validInput()
	in.Question = "  Best\n  language? "
	fields, closes := in.validate(now)
	if len(fields) != 0 {
		t.Fatal(fields)
	}
	if in.Question != "Best language?" || in.ResultsVisibility != VisibleAlways {
		t.Errorf("got %+v", in)
	}
	if closes.Location() != time.UTC {
		t.Error("closing time should be stored in UTC")
	}
}

func TestCanSeeResults(t *testing.T) {
	open := live.Meta{Status: StatusOpen, ClosesAt: now.Add(time.Hour)}
	closed := live.Meta{Status: StatusClosed, ClosesAt: now.Add(time.Hour)}
	expired := live.Meta{Status: StatusOpen, ClosesAt: now.Add(-time.Second)}

	with := func(m live.Meta, v string) live.Meta { m.Visibility = v; return m }
	cases := []struct {
		name               string
		meta               live.Meta
		owner, voted, want bool
	}{
		{"always", with(open, VisibleAlways), false, false, true},
		{"after vote, not voted", with(open, VisibleAfterVote), false, false, false},
		{"after vote, voted", with(open, VisibleAfterVote), false, true, true},
		{"after close, voted", with(open, VisibleAfterClose), false, true, false},
		{"after close, closed", with(closed, VisibleAfterClose), false, false, true},
		{"after close, expired", with(expired, VisibleAfterClose), false, false, true},
		{"owner always", with(open, VisibleAfterClose), true, false, true},
	}
	for _, tc := range cases {
		if got := CanSeeResults(tc.meta, now, tc.owner, tc.voted); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestEffectiveStatus(t *testing.T) {
	p := &Poll{Status: StatusOpen, ClosesAt: now}
	if p.EffectiveStatus(now) != StatusClosed {
		t.Error("poll at its closing instant should read as closed")
	}
	if p.EffectiveStatus(now.Add(-time.Second)) != StatusOpen {
		t.Error("poll before closing should be open")
	}
}
