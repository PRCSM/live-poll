// Package store opens the MongoDB and Redis connections and creates indexes.
package store

import (
	"context"
	"errors"
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
	// A fresh options builder per index: the driver records the index name on the
	// builder, so sharing one would give every index the same name.
	unique := func(name string) *options.IndexOptionsBuilder { return options.Index().SetUnique(true).SetName(name) }
	specs := map[string][]mongo.IndexModel{
		"users": {
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: unique("email_unique")},
		},
		"polls": {
			{Keys: bson.D{{Key: "slug", Value: 1}}, Options: unique("slug_unique")},
			{Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("owner_newest")},
		},
		"votes": {
			// The vote dedupe guarantee in Mongo: replays from the Redis stream are no-ops.
			{Keys: bson.D{{Key: "pollId", Value: 1}, {Key: "voterKey", Value: 1}}, Options: unique("poll_voter_unique")},
		},
	}
	for coll, models := range specs {
		iv := db.Collection(coll).Indexes()
		for _, m := range models {
			if err := createIndex(ctx, iv, m); err != nil {
				return fmt.Errorf("indexes on %s: %w", coll, err)
			}
		}
	}
	return nil
}

// createIndex creates m, replacing an index on the same keys whose name or options
// differ (left behind by an earlier version of this service).
func createIndex(ctx context.Context, iv mongo.IndexView, m mongo.IndexModel) error {
	_, err := iv.CreateOne(ctx, m)
	var ce mongo.CommandError
	if !errors.As(err, &ce) || (ce.Code != codeIndexOptionsConflict && ce.Code != codeIndexKeySpecsConflict) {
		return err
	}
	name, err := indexWithKeys(ctx, iv, m.Keys.(bson.D))
	if err != nil {
		return err
	}
	if err := iv.DropOne(ctx, name); err != nil {
		return fmt.Errorf("drop stale index %s: %w", name, err)
	}
	_, err = iv.CreateOne(ctx, m)
	return err
}

const (
	codeIndexOptionsConflict  = 85
	codeIndexKeySpecsConflict = 86
)

func indexWithKeys(ctx context.Context, iv mongo.IndexView, keys bson.D) (string, error) {
	cur, err := iv.List(ctx)
	if err != nil {
		return "", err
	}
	var specs []struct {
		Name string `bson:"name"`
		Key  bson.D `bson:"key"`
	}
	if err := cur.All(ctx, &specs); err != nil {
		return "", err
	}
	for _, s := range specs {
		if sameKeys(s.Key, keys) {
			return s.Name, nil
		}
	}
	return "", errors.New("conflicting index not found")
}

// sameKeys compares key specs; Mongo may return 1 as int32 where we wrote int.
func sameKeys(a, b bson.D) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || fmt.Sprint(a[i].Value) != fmt.Sprint(b[i].Value) {
			return false
		}
	}
	return true
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
