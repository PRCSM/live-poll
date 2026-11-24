package vote

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Vote struct {
	ID        bson.ObjectID  `bson:"_id"`
	PollID    bson.ObjectID  `bson:"pollId"`
	OptionIDs []string       `bson:"optionIds"`
	VoterKey  string         `bson:"voterKey"`
	IPHash    string         `bson:"ipHash"`
	UserID    *bson.ObjectID `bson:"userId,omitempty"`
	CreatedAt time.Time      `bson:"createdAt"`
}

type Store struct{ coll *mongo.Collection }

func NewStore(db *mongo.Database) *Store { return &Store{coll: db.Collection("votes")} }

// Insert is idempotent: the unique (pollId, voterKey) index turns a replayed
// queue entry into a no-op instead of a second vote.
func (s *Store) Insert(ctx context.Context, v *Vote) error {
	_, err := s.coll.InsertOne(ctx, v)
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

// Tally returns per-option counts and every voter key, used to rebuild Redis.
func (s *Store) Tally(ctx context.Context, pollID bson.ObjectID) (map[string]int64, []string, error) {
	cur, err := s.coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"pollId": pollID}}},
		{{Key: "$unwind", Value: "$optionIds"}},
		{{Key: "$group", Value: bson.M{"_id": "$optionIds", "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, nil, err
	}
	var rows []struct {
		ID string `bson:"_id"`
		N  int64  `bson:"n"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.ID] = r.N
	}

	keysCur, err := s.coll.Find(ctx, bson.M{"pollId": pollID}, options.Find().SetProjection(bson.M{"voterKey": 1, "_id": 0}))
	if err != nil {
		return nil, nil, err
	}
	var keyDocs []struct {
		VoterKey string `bson:"voterKey"`
	}
	if err := keysCur.All(ctx, &keyDocs); err != nil {
		return nil, nil, err
	}
	keys := make([]string, len(keyDocs))
	for i, d := range keyDocs {
		keys[i] = d.VoterKey
	}
	return counts, keys, nil
}

// Each streams a poll's votes oldest first, for CSV export.
func (s *Store) Each(ctx context.Context, pollID bson.ObjectID, fn func(Vote) error) error {
	cur, err := s.coll.Find(ctx, bson.M{"pollId": pollID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var v Vote
		if err := cur.Decode(&v); err != nil {
			return err
		}
		if err := fn(v); err != nil {
			return err
		}
	}
	return cur.Err()
}
