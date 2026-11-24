// Package store opens the MongoDB and Redis connections and creates indexes.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func OpenMongo(ctx context.Context, uri, dbName string) (*mongo.Client, *mongo.Database, error) {
	client, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetTimeout(10 * time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("mongo connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client, client.Database(dbName), nil
}

func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	// A fresh options builder per index: the driver records the generated index name
	// on the builder, so sharing one would give every index the same name.
	unique := func() *options.IndexOptionsBuilder { return options.Index().SetUnique(true) }
	specs := map[string][]mongo.IndexModel{
		"users": {
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: unique()},
		},
		"polls": {
			{Keys: bson.D{{Key: "slug", Value: 1}}, Options: unique()},
			{Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "_id", Value: -1}}},
		},
		"votes": {
			// The vote dedupe guarantee in Mongo: replays from the Redis stream are no-ops.
			{Keys: bson.D{{Key: "pollId", Value: 1}, {Key: "voterKey", Value: 1}}, Options: unique()},
		},
	}
	for coll, models := range specs {
		if _, err := db.Collection(coll).Indexes().CreateMany(ctx, models); err != nil {
			return fmt.Errorf("indexes on %s: %w", coll, err)
		}
	}
	return nil
}

func OpenRedis(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return rdb, nil
}
