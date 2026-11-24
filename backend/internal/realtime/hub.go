// Package realtime fans poll events out to Server-Sent Events clients.
//
// Each API instance holds one Redis pattern subscription for all polls and
// dispatches to its local clients, so the number of Redis connections does not
// grow with the number of viewers, and votes cast on any instance reach viewers
// on every instance.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/PRCSM/live-poll/backend/internal/live"
)

const (
	clientBuffer  = 16
	flushInterval = 100 * time.Millisecond
	viewersEvery  = 5 * time.Second
	maxPerIP      = 20
)

type client struct {
	pollID string
	events chan live.Event
	// lagged is set when the client could not keep up and events were dropped;
	// its writer then sends a fresh snapshot instead of the missed deltas.
	lagged atomic.Bool
	kick   chan struct{}
}

func (c *client) send(e live.Event) {
	select {
	case c.events <- e:
	default:
		c.markLagged()
	}
}

func (c *client) markLagged() {
	c.lagged.Store(true)
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

type Hub struct {
	rdb   redis.UniversalClient
	state *live.State

	mu      sync.Mutex
	clients map[string]map[*client]struct{}
	perIP   map[string]int
	// pending holds the newest counts event per poll; bursts of votes are
	// coalesced into at most one broadcast per flushInterval.
	pending  map[string]live.Event
	lastSeen map[string]int64

	closing chan struct{}
}

func NewHub(rdb redis.UniversalClient, state *live.State) *Hub {
	return &Hub{
		rdb: rdb, state: state,
		clients:  map[string]map[*client]struct{}{},
		perIP:    map[string]int{},
		pending:  map[string]live.Event{},
		lastSeen: map[string]int64{},
		closing:  make(chan struct{}),
	}
}

// Closing is closed when the server shuts down so open streams end promptly.
func (h *Hub) Closing() <-chan struct{} { return h.closing }

func (h *Hub) Close() { close(h.closing) }

func (h *Hub) join(pollID, ip string) (*client, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perIP[ip] >= maxPerIP {
		return nil, false
	}
	h.perIP[ip]++
	c := &client{pollID: pollID, events: make(chan live.Event, clientBuffer), kick: make(chan struct{}, 1)}
	if h.clients[pollID] == nil {
		h.clients[pollID] = map[*client]struct{}{}
	}
	h.clients[pollID][c] = struct{}{}
	return c, true
}

func (h *Hub) leave(c *client, ip string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perIP[ip]--; h.perIP[ip] <= 0 {
		delete(h.perIP, ip)
	}
	delete(h.clients[c.pollID], c)
	if len(h.clients[c.pollID]) == 0 {
		delete(h.clients, c.pollID)
		delete(h.pending, c.pollID)
		delete(h.lastSeen, c.pollID)
	}
}

func (h *Hub) broadcast(pollID string, e live.Event) {
	for c := range h.clients[pollID] {
		c.send(e)
	}
}

func (h *Hub) dispatch(pollID string, e live.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients[pollID]) == 0 {
		return
	}
	if e.Type == "counts" {
		if prev, ok := h.pending[pollID]; !ok || e.Ver > prev.Ver {
			h.pending[pollID] = e
		}
		return
	}
	// Status changes must not overtake counts that were cast before them.
	if p, ok := h.pending[pollID]; ok {
		h.broadcast(pollID, p)
		delete(h.pending, pollID)
	}
	h.broadcast(pollID, e)
}

func (h *Hub) flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, e := range h.pending {
		h.broadcast(id, e)
	}
	clear(h.pending)
}

// resyncAll is used after the Redis subscription reconnects: events may have been
// missed, so every client gets a fresh snapshot.
func (h *Hub) resyncAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.clients {
		for c := range set {
			c.markLagged()
		}
	}
}

func (h *Hub) Run(ctx context.Context) {
	go h.runTickers(ctx)

	ps := h.rdb.PSubscribe(ctx, live.ChannelPattern)
	defer ps.Close()
	subscribedOnce := false
	for {
		msg, err := ps.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("realtime: subscription error, reconnecting", "err", err)
			time.Sleep(time.Second)
			continue
		}
		switch m := msg.(type) {
		case *redis.Subscription:
			if subscribedOnce {
				h.resyncAll()
			}
			subscribedOnce = true
		case *redis.Message:
			var e live.Event
			if err := json.Unmarshal([]byte(m.Payload), &e); err != nil {
				slog.Warn("realtime: bad event", "channel", m.Channel, "err", err)
				continue
			}
			h.dispatch(live.PollID(m.Channel), e)
		}
	}
}

func (h *Hub) runTickers(ctx context.Context) {
	flush := time.NewTicker(flushInterval)
	viewers := time.NewTicker(viewersEvery)
	defer flush.Stop()
	defer viewers.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-flush.C:
			h.flush()
		case <-viewers.C:
			h.pushViewers(ctx)
		}
	}
}

func (h *Hub) pushViewers(ctx context.Context) {
	h.mu.Lock()
	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	h.mu.Unlock()

	for _, id := range ids {
		n, err := h.state.Viewers(ctx, id)
		if err != nil {
			continue
		}
		h.mu.Lock()
		if h.lastSeen[id] != n {
			h.lastSeen[id] = n
			h.broadcast(id, live.Event{Type: "viewers", Viewers: n})
		}
		h.mu.Unlock()
	}
}
