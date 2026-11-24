package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/PRCSM/live-poll/backend/internal/auth"
	"github.com/PRCSM/live-poll/backend/internal/config"
	"github.com/PRCSM/live-poll/backend/internal/httpx"
	"github.com/PRCSM/live-poll/backend/internal/live"
	"github.com/PRCSM/live-poll/backend/internal/poll"
	"github.com/PRCSM/live-poll/backend/internal/ratelimit"
	"github.com/PRCSM/live-poll/backend/internal/realtime"
	"github.com/PRCSM/live-poll/backend/internal/store"
	"github.com/PRCSM/live-poll/backend/internal/vote"
	"github.com/PRCSM/live-poll/backend/internal/voter"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Production() {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mongoClient, db, err := store.OpenMongo(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		return err
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	if err := store.EnsureIndexes(ctx, db); err != nil {
		return err
	}
	rdb, err := store.OpenRedis(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	users := auth.NewUsers(db)
	polls := poll.NewStore(db)
	votes := vote.NewStore(db)
	limiter := ratelimit.New(rdb)
	signer := voter.NewSigner(cfg.VoterSecret)
	state := live.NewState(rdb, liveSource{polls: polls, votes: votes})
	hub := realtime.NewHub(rdb, state)

	authH, err := auth.NewHandler(users, auth.NewTokens(cfg.JWTSecret), limiter)
	if err != nil {
		return err
	}
	pollH := poll.NewHandler(polls, state, authH, signer, limiter)

	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return err
	}
	r.TrustedPlatform = cfg.ClientIPHeader
	r.Use(httpx.Recover(), httpx.RequestID(), httpx.Logger(), httpx.SecurityHeaders(cfg.Production()), httpx.CORS(cfg.FrontendOrigin))
	r.GET("/healthz", health(mongoClient, rdb))

	api := r.Group("/api", httpx.ServerTime(), httpx.BodyLimit(16<<10))
	authH.Register(api)
	pollH.Register(api)
	vote.NewHandler(polls, votes, state, authH, signer, limiter).Register(api)
	realtime.NewStreamHandler(hub, polls, state, signer).Register(api)

	go hub.Run(ctx)
	go vote.NewWorker(rdb, votes).Run(ctx)
	go pollH.RunAutoClose(ctx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout: it would cut long-lived event streams. Normal handlers are
		// bounded by the Mongo and Redis client timeouts instead.
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	hub.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func health(mc *mongo.Client, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		res := gin.H{"status": "ok", "mongo": "ok", "redis": "ok"}
		code := http.StatusOK
		if err := mc.Ping(ctx, nil); err != nil {
			res["mongo"], res["status"], code = "down", "degraded", http.StatusServiceUnavailable
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			res["redis"], res["status"], code = "down", "degraded", http.StatusServiceUnavailable
		}
		c.JSON(code, res)
	}
}

// liveSource lets the live package rebuild Redis state from MongoDB.
type liveSource struct {
	polls *poll.Store
	votes *vote.Store
}

func (s liveSource) LoadMeta(ctx context.Context, id string) (live.Meta, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return live.Meta{}, live.ErrPollGone
	}
	p, err := s.polls.ByID(ctx, oid)
	if errors.Is(err, poll.ErrNotFound) {
		return live.Meta{}, live.ErrPollGone
	}
	if err != nil {
		return live.Meta{}, err
	}
	return p.Meta(), nil
}

func (s liveSource) VoteState(ctx context.Context, id string) (map[string]int64, []string, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil, err
	}
	return s.votes.Tally(ctx, oid)
}
