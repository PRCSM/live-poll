package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PRCSM/live-poll/backend/internal/apperr"
	"github.com/PRCSM/live-poll/backend/internal/httpx"
	"github.com/PRCSM/live-poll/backend/internal/live"
	"github.com/PRCSM/live-poll/backend/internal/poll"
	"github.com/PRCSM/live-poll/backend/internal/voter"
)

const heartbeatEvery = 15 * time.Second

var errTooManyStreams = apperr.New(http.StatusTooManyRequests, "too_many_streams", "Too many live connections from your network.")

type StreamHandler struct {
	hub    *Hub
	polls  *poll.Store
	state  *live.State
	signer *voter.Signer
}

func NewStreamHandler(hub *Hub, polls *poll.Store, state *live.State, signer *voter.Signer) *StreamHandler {
	return &StreamHandler{hub: hub, polls: polls, state: state, signer: signer}
}

func (h *StreamHandler) Register(api *gin.RouterGroup) {
	api.GET("/polls/:slug/stream", h.stream)
}

// snapshotEvent is the first event on every stream and the payload of a resync.
type snapshotEvent struct {
	Type     string           `json:"type"`
	Ver      int64            `json:"ver"`
	Counts   map[string]int64 `json:"counts,omitempty"`
	Total    int64            `json:"total"`
	Status   string           `json:"status"`
	ClosesAt time.Time        `json:"closesAt"`
	Hidden   bool             `json:"resultsHidden"`
	Viewers  int64            `json:"viewers"`
}

func (h *StreamHandler) stream(c *gin.Context) {
	ctx := c.Request.Context()
	p, meta, err := poll.Load(ctx, h.polls, h.state, c.Param("slug"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	id := p.ID.Hex()
	ip := c.ClientIP()
	cl, ok := h.hub.join(id, ip)
	if !ok {
		httpx.Fail(c, errTooManyStreams)
		return
	}
	defer h.hub.leave(cl, ip)

	connID := make([]byte, 8)
	_, _ = rand.Read(connID)
	conn := hex.EncodeToString(connID)
	_ = h.state.TouchViewer(ctx, id, conn)
	defer func() {
		// The request context is already cancelled once the client is gone.
		leaveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = h.state.LeaveViewer(leaveCtx, id, conn)
	}()

	unlocked := h.signer.ValidStreamToken(id, c.Query("st"))
	visible := func(m live.Meta) bool { return unlocked || poll.CanSeeResults(m, time.Now(), false, false) }

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no") // stop proxies from buffering the stream
	c.Status(http.StatusOK)

	w := c.Writer
	if _, err := io.WriteString(w, "retry: 3000\n\n"); err != nil {
		return
	}
	// Subscribed before the snapshot is read, so nothing between the two is missed;
	// the client drops anything older than the snapshot by version.
	if err := h.writeSnapshot(c, id, meta, visible(meta)); err != nil {
		return
	}

	heartbeat := time.NewTicker(heartbeatEvery)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.hub.Closing():
			return
		case <-heartbeat.C:
			_ = h.state.TouchViewer(ctx, id, conn)
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			w.Flush()
		case <-cl.kick:
			if !cl.lagged.Swap(false) {
				continue
			}
			for len(cl.events) > 0 {
				<-cl.events
			}
			m, err := h.state.Meta(ctx, id)
			if errors.Is(err, live.ErrPollGone) {
				_ = writeEvent(w, live.Event{Type: "deleted"})
				return
			}
			if err != nil {
				return // the client reconnects and gets a fresh snapshot
			}
			meta = m
			if err := h.writeSnapshot(c, id, meta, visible(meta)); err != nil {
				return
			}
		case e := <-cl.events:
			switch e.Type {
			case "deleted":
				_ = writeEvent(w, e)
				return
			case "status":
				wasVisible := visible(meta)
				meta.Status = e.Status
				if e.ClosesAt != nil {
					meta.ClosesAt = *e.ClosesAt
				}
				if err := writeEvent(w, e); err != nil {
					return
				}
				// Results hidden until close become visible now: send them.
				if !wasVisible && visible(meta) {
					if err := h.writeSnapshot(c, id, meta, true); err != nil {
						return
					}
				}
			case "counts":
				if !visible(meta) {
					e.Counts = nil
				}
				if err := writeEvent(w, e); err != nil {
					return
				}
			default:
				if err := writeEvent(w, e); err != nil {
					return
				}
			}
		}
	}
}

func (h *StreamHandler) writeSnapshot(c *gin.Context, id string, meta live.Meta, visible bool) error {
	snap, err := h.state.Snapshot(c.Request.Context(), id)
	if err != nil {
		slog.Warn("stream snapshot", "poll", id, "err", err)
		return err
	}
	viewers, _ := h.state.Viewers(c.Request.Context(), id)
	status := poll.StatusClosed
	if meta.Open(time.Now()) {
		status = poll.StatusOpen
	}
	se := snapshotEvent{
		Type: "snapshot", Ver: snap.Ver, Total: snap.Total,
		Status: status, ClosesAt: meta.ClosesAt.UTC(), Hidden: !visible, Viewers: viewers,
	}
	if visible {
		se.Counts = snap.Counts
	}
	return writeEvent(c.Writer, se)
}

type flushWriter interface {
	io.Writer
	Flush()
}

func writeEvent(w flushWriter, e any) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	var typ string
	switch v := e.(type) {
	case live.Event:
		typ = v.Type
	case snapshotEvent:
		typ = v.Type
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, b); err != nil {
		return err
	}
	w.Flush()
	return nil
}
