package poll

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/PRCSM/live-poll/backend/internal/apperr"
	"github.com/PRCSM/live-poll/backend/internal/auth"
	"github.com/PRCSM/live-poll/backend/internal/httpx"
	"github.com/PRCSM/live-poll/backend/internal/live"
	"github.com/PRCSM/live-poll/backend/internal/ratelimit"
	"github.com/PRCSM/live-poll/backend/internal/voter"
)

const pageSize = 20

type Handler struct {
	store   *Store
	state   *live.State
	auth    *auth.Handler
	signer  *voter.Signer
	limiter *ratelimit.Limiter
	now     func() time.Time
}

func NewHandler(store *Store, state *live.State, a *auth.Handler, signer *voter.Signer, limiter *ratelimit.Limiter) *Handler {
	return &Handler{store: store, state: state, auth: a, signer: signer, limiter: limiter, now: time.Now}
}

func (h *Handler) Register(api *gin.RouterGroup) {
	my := api.Group("/my/polls", h.auth.Required())
	byUser := func(c *gin.Context) string { return auth.CurrentUser(c).ID.Hex() }
	my.POST("", h.limiter.Middleware("poll-create", 20, time.Hour, byUser), h.create)
	my.GET("", h.list)
	my.PATCH("/:id", h.update)
	my.DELETE("/:id", h.delete)

	api.GET("/polls/:slug", h.auth.Optional(), h.view)
}

// ownerPoll is the dashboard view of a poll.
type ownerPoll struct {
	*Poll
	Status     string `json:"status"`
	TotalVotes *int64 `json:"totalVotes"`
}

func (h *Handler) toOwner(p *Poll, totals map[string]int64) ownerPoll {
	op := ownerPoll{Poll: p, Status: p.EffectiveStatus(h.now())}
	if n, ok := totals[p.ID.Hex()]; ok {
		op.TotalVotes = &n
	}
	return op
}

func (h *Handler) create(c *gin.Context) {
	var in createInput
	if err := httpx.DecodeJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	now := h.now().UTC()
	fields, closesAt := in.validate(now)
	if len(fields) > 0 {
		httpx.Fail(c, apperr.Validation(fields))
		return
	}
	p := &Poll{
		ID:                bson.NewObjectID(),
		OwnerID:           auth.CurrentUser(c).ID,
		Question:          in.Question,
		Options:           buildOptions(in.Options),
		MultiChoice:       in.MultiChoice,
		ResultsVisibility: in.ResultsVisibility,
		RequireLogin:      in.RequireLogin,
		Status:            StatusOpen,
		ClosesAt:          closesAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := h.store.Insert(c.Request.Context(), p); err != nil {
		httpx.Fail(c, err)
		return
	}
	// MongoDB has the poll, so a Redis failure here is recoverable: the state is
	// rebuilt from Mongo on first access. Failing the request would invite a duplicate.
	if err := h.state.Init(c.Request.Context(), p.ID.Hex(), p.Meta()); err != nil {
		slog.Warn("init live state", "poll", p.ID.Hex(), "err", err)
	}
	c.JSON(http.StatusCreated, gin.H{"poll": h.toOwner(p, map[string]int64{p.ID.Hex(): 0})})
}

func (h *Handler) list(c *gin.Context) {
	var before *bson.ObjectID
	if raw := c.Query("before"); raw != "" {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			httpx.Fail(c, apperr.Validation(map[string]string{"before": "Invalid cursor."}))
			return
		}
		before = &id
	}
	polls, err := h.store.ListByOwner(c.Request.Context(), auth.CurrentUser(c).ID, before, pageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ids := make([]string, len(polls))
	for i := range polls {
		ids[i] = polls[i].ID.Hex()
	}
	totals, err := h.state.Totals(c.Request.Context(), ids)
	if err != nil {
		slog.Warn("load totals", "err", err)
	}
	out := make([]ownerPoll, len(polls))
	for i := range polls {
		out[i] = h.toOwner(&polls[i], totals)
	}
	var next *string
	if len(polls) == pageSize {
		cursor := polls[len(polls)-1].ID.Hex()
		next = &cursor
	}
	c.JSON(http.StatusOK, gin.H{"polls": out, "nextCursor": next})
}

func (h *Handler) ownedID(c *gin.Context) (bson.ObjectID, bool) {
	id, err := bson.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		httpx.Fail(c, apperr.ErrNotFound)
		return id, false
	}
	return id, true
}

func (h *Handler) update(c *gin.Context) {
	id, ok := h.ownedID(c)
	if !ok {
		return
	}
	var in struct {
		Status   string `json:"status"`
		ClosesAt string `json:"closesAt"`
	}
	if err := httpx.DecodeJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	owner := auth.CurrentUser(c).ID
	current, err := h.store.Owned(ctx, id, owner)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	now := h.now().UTC()
	var closesAt time.Time
	switch in.Status {
	case StatusClosed:
		if in.ClosesAt != "" {
			httpx.Fail(c, apperr.Validation(map[string]string{"closesAt": "Only set a closing time when reopening."}))
			return
		}
		closesAt = current.ClosesAt
		if now.Before(closesAt) {
			closesAt = now
		}
	case StatusOpen:
		t, msg := parseClosesAt(in.ClosesAt, now)
		if msg != "" {
			httpx.Fail(c, apperr.Validation(map[string]string{"closesAt": "To reopen, pick a new closing time. " + msg}))
			return
		}
		closesAt = t
	default:
		httpx.Fail(c, apperr.Validation(map[string]string{"status": "Status must be open or closed."}))
		return
	}

	p, err := h.store.SetStatus(ctx, id, owner, in.Status, closesAt)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.state.SetStatus(ctx, id.Hex(), p.Status, p.ClosesAt); err != nil {
		h.invalidate(ctx, id.Hex(), err)
		httpx.Fail(c, apperr.ErrUnavailable)
		return
	}
	totals, _ := h.state.Totals(ctx, []string{id.Hex()})
	c.JSON(http.StatusOK, gin.H{"poll": h.toOwner(p, totals)})
}

func (h *Handler) delete(c *gin.Context) {
	id, ok := h.ownedID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	err := h.store.Delete(ctx, id, auth.CurrentUser(c).ID)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(c, apperr.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.state.Remove(ctx, id.Hex()); err != nil {
		h.invalidate(ctx, id.Hex(), err)
	}
	c.Status(http.StatusNoContent)
}

// invalidate drops the Redis copy after a failed sync so it is rebuilt from Mongo.
func (h *Handler) invalidate(ctx context.Context, id string, cause error) {
	slog.Error("live state out of sync", "poll", id, "err", cause)
	if err := h.state.Invalidate(context.WithoutCancel(ctx), id); err != nil {
		slog.Error("invalidate live state", "poll", id, "err", err)
	}
}

// publicPoll is what any visitor sees.
type publicPoll struct {
	ID                string    `json:"id,omitempty"`
	Slug              string    `json:"slug"`
	Question          string    `json:"question"`
	Options           []Option  `json:"options"`
	MultiChoice       bool      `json:"multiChoice"`
	ResultsVisibility string    `json:"resultsVisibility"`
	RequireLogin      bool      `json:"requireLogin"`
	Status            string    `json:"status"`
	ClosesAt          time.Time `json:"closesAt"`
	CreatedAt         time.Time `json:"createdAt"`
	IsOwner           bool      `json:"isOwner"`
}

// Results carries counts only when the viewer may see them; Total is always shown.
type Results struct {
	Ver    int64            `json:"ver"`
	Counts map[string]int64 `json:"counts,omitempty"`
	Total  int64            `json:"total"`
}

func ResultsFrom(e live.Event, visible bool) Results {
	r := Results{Ver: e.Ver, Total: e.Total}
	if visible {
		r.Counts = e.Counts
	}
	return r
}

// Identify resolves who is voting: the account when the poll requires login,
// otherwise the signed device token, issuing a new one when it is missing or invalid.
func Identify(c *gin.Context, signer *voter.Signer, requireLogin bool) (key, newToken string) {
	if requireLogin {
		if u := auth.CurrentUser(c); u != nil {
			return signer.UserKey(u.ID.Hex()), ""
		}
		return "", ""
	}
	if id, ok := signer.ParseToken(c.GetHeader("X-Voter-Token")); ok {
		return signer.Key(id), ""
	}
	tok := signer.NewToken()
	id, _ := signer.ParseToken(tok)
	return signer.Key(id), tok
}

// ValidSlug rejects anything that could not be a slug before touching the database.
func ValidSlug(s string) bool {
	if len(s) != 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(slugAlphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}

// Load fetches a poll and its live metadata by slug, mapping every "gone" case to 404.
func Load(ctx context.Context, store *Store, state *live.State, slug string) (*Poll, live.Meta, error) {
	if !ValidSlug(slug) {
		return nil, live.Meta{}, apperr.ErrNotFound
	}
	p, err := store.BySlug(ctx, slug)
	if errors.Is(err, ErrNotFound) {
		return nil, live.Meta{}, apperr.ErrNotFound
	}
	if err != nil {
		return nil, live.Meta{}, err
	}
	m, err := state.Meta(ctx, p.ID.Hex())
	if errors.Is(err, live.ErrPollGone) {
		return nil, live.Meta{}, apperr.ErrNotFound
	}
	if err != nil {
		slog.Error("load live state", "poll", p.ID.Hex(), "err", err)
		return nil, live.Meta{}, apperr.ErrUnavailable
	}
	return p, m, nil
}

func (h *Handler) view(c *gin.Context) {
	ctx := c.Request.Context()
	p, meta, err := Load(ctx, h.store, h.state, c.Param("slug"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	id := p.ID.Hex()
	now := h.now()
	user := auth.CurrentUser(c)
	isOwner := user != nil && user.ID == p.OwnerID

	key, newToken := Identify(c, h.signer, p.RequireLogin)
	hasVoted := false
	if key != "" && newToken == "" {
		if hasVoted, err = h.state.HasVoted(ctx, id, key); err != nil {
			httpx.Fail(c, apperr.ErrUnavailable)
			return
		}
	}
	visible := CanSeeResults(meta, now, isOwner, hasVoted)
	snap, err := h.state.Snapshot(ctx, id)
	if err != nil {
		httpx.Fail(c, apperr.ErrUnavailable)
		return
	}
	viewers, _ := h.state.Viewers(ctx, id)

	pp := publicPoll{
		Slug: p.Slug, Question: p.Question, Options: p.Options, MultiChoice: p.MultiChoice,
		ResultsVisibility: p.ResultsVisibility, RequireLogin: p.RequireLogin,
		Status: liveStatus(meta, now), ClosesAt: meta.ClosesAt.UTC(), CreatedAt: p.CreatedAt, IsOwner: isOwner,
	}
	if isOwner {
		pp.ID = id
	}
	resp := gin.H{
		"poll":          pp,
		"results":       ResultsFrom(snap, visible),
		"resultsHidden": !visible,
		"hasVoted":      hasVoted,
		"viewers":       viewers,
	}
	if newToken != "" {
		resp["voterToken"] = newToken
	}
	if visible {
		resp["streamToken"] = h.signer.StreamToken(id)
	}
	c.JSON(http.StatusOK, resp)
}

func liveStatus(m live.Meta, now time.Time) string {
	if m.Open(now) {
		return StatusOpen
	}
	return StatusClosed
}

// RunAutoClose closes polls when their time is up. Every instance runs it; the
// ZREM claim in Redis makes sure exactly one of them closes each poll.
func (h *Handler) RunAutoClose(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.closeDue(ctx)
		}
	}
}

func (h *Handler) closeDue(ctx context.Context) {
	due, err := h.state.DueForClose(ctx)
	if err != nil {
		slog.Warn("auto-close: list due", "err", err)
		return
	}
	for _, id := range due {
		claimed, err := h.state.ClaimClose(ctx, id)
		if err != nil || !claimed {
			continue
		}
		oid, err := bson.ObjectIDFromHex(id)
		if err != nil {
			continue
		}
		p, err := h.store.CloseIfOpen(ctx, oid)
		if errors.Is(err, ErrNotFound) {
			continue // deleted, or closed manually already
		}
		if err != nil {
			slog.Error("auto-close", "poll", id, "err", err)
			_ = h.state.Schedule(ctx, id, h.now().Add(5*time.Second))
			continue
		}
		if err := h.state.SetStatus(ctx, id, p.Status, p.ClosesAt); err != nil {
			h.invalidate(ctx, id, err)
		}
	}
}
