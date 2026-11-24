package vote

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/PRCSM/live-poll/backend/internal/apperr"
	"github.com/PRCSM/live-poll/backend/internal/auth"
	"github.com/PRCSM/live-poll/backend/internal/httpx"
	"github.com/PRCSM/live-poll/backend/internal/live"
	"github.com/PRCSM/live-poll/backend/internal/poll"
	"github.com/PRCSM/live-poll/backend/internal/ratelimit"
	"github.com/PRCSM/live-poll/backend/internal/voter"
)

var (
	errAlreadyVoted  = apperr.New(http.StatusConflict, "already_voted", "You have already voted in this poll.")
	errPollClosed    = apperr.New(http.StatusConflict, "poll_closed", "This poll is closed.")
	errLoginRequired = apperr.New(http.StatusUnauthorized, "login_required", "Sign in to vote in this poll.")
)

type Handler struct {
	polls   *poll.Store
	votes   *Store
	state   *live.State
	auth    *auth.Handler
	signer  *voter.Signer
	limiter *ratelimit.Limiter
	now     func() time.Time
}

func NewHandler(polls *poll.Store, votes *Store, state *live.State, a *auth.Handler, signer *voter.Signer, limiter *ratelimit.Limiter) *Handler {
	return &Handler{polls: polls, votes: votes, state: state, auth: a, signer: signer, limiter: limiter, now: time.Now}
}

func (h *Handler) Register(api *gin.RouterGroup) {
	api.POST("/polls/:slug/votes",
		h.limiter.Middleware("vote-ip", 60, time.Minute, ratelimit.ByIP),
		h.limiter.Middleware("vote-ip-poll", 10, time.Minute, func(c *gin.Context) string {
			return c.ClientIP() + ":" + c.Param("slug")
		}),
		h.auth.Optional(),
		h.cast)
	api.GET("/my/polls/:id/export.csv", h.auth.Required(), h.export)
}

type castInput struct {
	OptionIDs []string `json:"optionIds"`
}

func validateChoice(in castInput, m live.Meta) map[string]string {
	n := len(in.OptionIDs)
	switch {
	case n == 0:
		return map[string]string{"optionIds": "Pick an option."}
	case !m.MultiChoice && n > 1:
		return map[string]string{"optionIds": "This poll allows only one choice."}
	case n > len(m.OptionIDs):
		return map[string]string{"optionIds": "Too many options selected."}
	}
	seen := map[string]bool{}
	for _, id := range in.OptionIDs {
		if seen[id] {
			return map[string]string{"optionIds": "An option was selected twice."}
		}
		if !m.HasOption(id) {
			return map[string]string{"optionIds": "That option is not part of this poll."}
		}
		seen[id] = true
	}
	return nil
}

func (h *Handler) cast(c *gin.Context) {
	ctx := c.Request.Context()
	var in castInput
	if err := httpx.DecodeJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, meta, err := poll.Load(ctx, h.polls, h.state, c.Param("slug"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if !meta.Open(h.now()) {
		httpx.Fail(c, errPollClosed)
		return
	}
	if fields := validateChoice(in, meta); fields != nil {
		httpx.Fail(c, apperr.Validation(fields))
		return
	}

	user := auth.CurrentUser(c)
	key, newToken := poll.Identify(c, h.signer, p.RequireLogin)
	if key == "" {
		httpx.Fail(c, errLoginRequired)
		return
	}
	b := live.Ballot{PollID: p.ID.Hex(), VoterKey: key, OptionIDs: in.OptionIDs, IPHash: h.signer.IPHash(c.ClientIP())}
	if user != nil {
		b.UserID = user.ID.Hex()
	}

	e, err := h.state.CastVote(ctx, b)
	switch {
	case errors.Is(err, live.ErrAlreadyVoted):
		httpx.Fail(c, errAlreadyVoted)
		return
	case errors.Is(err, live.ErrClosed):
		httpx.Fail(c, errPollClosed)
		return
	case errors.Is(err, live.ErrPollGone):
		httpx.Fail(c, apperr.ErrNotFound)
		return
	case err != nil:
		httpx.Fail(c, apperr.ErrUnavailable)
		return
	}

	isOwner := user != nil && user.ID == p.OwnerID
	visible := poll.CanSeeResults(meta, h.now(), isOwner, true)
	resp := gin.H{"results": poll.ResultsFrom(e, visible), "resultsHidden": !visible, "hasVoted": true}
	if newToken != "" {
		resp["voterToken"] = newToken
	}
	if visible {
		resp["streamToken"] = h.signer.StreamToken(p.ID.Hex())
	}
	c.JSON(http.StatusCreated, resp)
}

// csvSafe stops spreadsheet apps from running cells as formulas (CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (h *Handler) export(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := bson.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	p, err := h.polls.Owned(ctx, id, auth.CurrentUser(c).ID)
	if errors.Is(err, poll.ErrNotFound) {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	snap, err := h.state.Snapshot(ctx, id.Hex())
	if err != nil {
		httpx.Fail(c, apperr.ErrUnavailable)
		return
	}

	text := make(map[string]string, len(p.Options))
	for _, o := range p.Options {
		text[o.ID] = o.Text
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="poll-`+p.Slug+`.csv"`)
	c.Status(http.StatusOK)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"question", csvSafe(p.Question)})
	_ = w.Write([]string{"total votes", strconv.FormatInt(snap.Total, 10)})
	_ = w.Write(nil)
	_ = w.Write([]string{"option", "votes"})
	for _, o := range p.Options {
		_ = w.Write([]string{csvSafe(o.Text), strconv.FormatInt(snap.Counts[o.ID], 10)})
	}
	_ = w.Write(nil)
	_ = w.Write([]string{"voted at (UTC)", "choice"})
	err = h.votes.Each(ctx, id, func(v Vote) error {
		names := make([]string, len(v.OptionIDs))
		for i, oid := range v.OptionIDs {
			names[i] = csvSafe(text[oid])
		}
		return w.Write([]string{v.CreatedAt.Format(time.RFC3339), strings.Join(names, "; ")})
	})
	w.Flush()
	if err == nil {
		err = w.Error()
	}
	if err != nil {
		// Headers are already sent, so the client sees a truncated file; record why.
		_ = c.Error(err)
	}
}
