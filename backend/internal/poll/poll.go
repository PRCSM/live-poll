package poll

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/PRCSM/live-poll/backend/internal/live"
	"github.com/PRCSM/live-poll/backend/internal/textx"
)

const (
	StatusOpen   = "open"
	StatusClosed = "closed"

	VisibleAlways     = "always"
	VisibleAfterVote  = "after_vote"
	VisibleAfterClose = "after_close"

	maxQuestionRunes = 280
	maxOptionRunes   = 100
	minOptions       = 2
	maxOptions       = 10
	minDuration      = time.Minute
	maxDuration      = 30 * 24 * time.Hour
)

type Option struct {
	ID   string `bson:"id" json:"id"`
	Text string `bson:"text" json:"text"`
}

type Poll struct {
	ID                bson.ObjectID `bson:"_id" json:"id"`
	Slug              string        `bson:"slug" json:"slug"`
	OwnerID           bson.ObjectID `bson:"ownerId" json:"-"`
	Question          string        `bson:"question" json:"question"`
	Options           []Option      `bson:"options" json:"options"`
	MultiChoice       bool          `bson:"multiChoice" json:"multiChoice"`
	ResultsVisibility string        `bson:"resultsVisibility" json:"resultsVisibility"`
	RequireLogin      bool          `bson:"requireLogin" json:"requireLogin"`
	Status            string        `bson:"status" json:"status"`
	ClosesAt          time.Time     `bson:"closesAt" json:"closesAt"`
	CreatedAt         time.Time     `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time     `bson:"updatedAt" json:"updatedAt"`
}

// EffectiveStatus treats a poll past its closing time as closed even if the
// auto-closer has not run yet.
func (p *Poll) EffectiveStatus(now time.Time) string {
	if p.Status == StatusOpen && !now.Before(p.ClosesAt) {
		return StatusClosed
	}
	return p.Status
}

func (p *Poll) Meta() live.Meta {
	ids := make([]string, len(p.Options))
	for i, o := range p.Options {
		ids[i] = o.ID
	}
	return live.Meta{
		Status:       p.Status,
		ClosesAt:     p.ClosesAt,
		OptionIDs:    ids,
		MultiChoice:  p.MultiChoice,
		Visibility:   p.ResultsVisibility,
		RequireLogin: p.RequireLogin,
		OwnerID:      p.OwnerID.Hex(),
	}
}

// CanSeeResults decides whether counts may be sent to a viewer. It is enforced on
// the server for both the REST response and the stream, not just hidden in the UI.
func CanSeeResults(m live.Meta, now time.Time, isOwner, hasVoted bool) bool {
	switch {
	case isOwner, m.Visibility == VisibleAlways, !m.Open(now):
		return true
	case m.Visibility == VisibleAfterVote:
		return hasVoted
	}
	return false
}

type createInput struct {
	Question          string   `json:"question"`
	Options           []string `json:"options"`
	MultiChoice       bool     `json:"multiChoice"`
	ResultsVisibility string   `json:"resultsVisibility"`
	RequireLogin      bool     `json:"requireLogin"`
	ClosesAt          string   `json:"closesAt"`
}

// validate cleans the input in place and returns field errors plus the parsed closing time.
func (in *createInput) validate(now time.Time) (map[string]string, time.Time) {
	fields := map[string]string{}

	in.Question = textx.Clean(in.Question)
	if n := utf8.RuneCountInString(in.Question); n == 0 || n > maxQuestionRunes {
		fields["question"] = "Question must be 1 to 280 characters."
	}

	if len(in.Options) < minOptions || len(in.Options) > maxOptions {
		fields["options"] = "Add between 2 and 10 options."
	} else {
		seen := map[string]int{}
		for i := range in.Options {
			in.Options[i] = textx.Clean(in.Options[i])
			key := "options." + strconv.Itoa(i)
			n := utf8.RuneCountInString(in.Options[i])
			if n == 0 || n > maxOptionRunes {
				fields[key] = "Each option must be 1 to 100 characters."
				continue
			}
			lower := strings.ToLower(in.Options[i])
			if first, dup := seen[lower]; dup {
				fields[key] = "Duplicate of option " + strconv.Itoa(first+1) + "."
				continue
			}
			seen[lower] = i
		}
	}

	switch in.ResultsVisibility {
	case "":
		in.ResultsVisibility = VisibleAlways
	case VisibleAlways, VisibleAfterVote, VisibleAfterClose:
	default:
		fields["resultsVisibility"] = "Choose always, after_vote or after_close."
	}

	closesAt, err := parseClosesAt(in.ClosesAt, now)
	if err != "" {
		fields["closesAt"] = err
	}
	return fields, closesAt
}

func parseClosesAt(s string, now time.Time) (time.Time, string) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, "Closing time must be an RFC 3339 timestamp."
	}
	t = t.UTC()
	switch {
	case t.Before(now.Add(minDuration)):
		return t, "Closing time must be at least 1 minute from now."
	case t.After(now.Add(maxDuration)):
		return t, "Closing time must be within 30 days."
	}
	return t, ""
}

func buildOptions(texts []string) []Option {
	opts := make([]Option, len(texts))
	for i, t := range texts {
		opts[i] = Option{ID: "o" + strconv.Itoa(i+1), Text: t}
	}
	return opts
}
