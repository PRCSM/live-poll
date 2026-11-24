package poll

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrNotFound = errors.New("poll not found")

type Store struct {
	polls *mongo.Collection
	votes *mongo.Collection
}

func NewStore(db *mongo.Database) *Store {
	return &Store{polls: db.Collection("polls"), votes: db.Collection("votes")}
}

const slugAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newSlug skips look-alike characters (0/O, 1/l/I) so links read aloud correctly.
func newSlug() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}

// Insert stores p, retrying with a new slug on the rare collision.
func (s *Store) Insert(ctx context.Context, p *Poll) error {
	for attempt := 0; attempt < 3; attempt++ {
		p.Slug = newSlug()
		_, err := s.polls.InsertOne(ctx, p)
		if !mongo.IsDuplicateKeyError(err) {
			return err
		}
	}
	return errors.New("could not allocate a unique slug")
}

func (s *Store) BySlug(ctx context.Context, slug string) (*Poll, error) {
	return s.findOne(ctx, bson.M{"slug": slug})
}

func (s *Store) ByID(ctx context.Context, id bson.ObjectID) (*Poll, error) {
	return s.findOne(ctx, bson.M{"_id": id})
}

func (s *Store) Owned(ctx context.Context, id, owner bson.ObjectID) (*Poll, error) {
	return s.findOne(ctx, bson.M{"_id": id, "ownerId": owner})
}

func (s *Store) findOne(ctx context.Context, filter bson.M) (*Poll, error) {
	var p Poll
	err := s.polls.FindOne(ctx, filter).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByOwner pages newest first; before is the last id of the previous page.
func (s *Store) ListByOwner(ctx context.Context, owner bson.ObjectID, before *bson.ObjectID, limit int) ([]Poll, error) {
	filter := bson.M{"ownerId": owner}
	if before != nil {
		filter["_id"] = bson.M{"$lt": *before}
	}
	cur, err := s.polls.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	polls := []Poll{}
	return polls, cur.All(ctx, &polls)
}

// SetStatus updates an owned poll and returns the new document.
func (s *Store) SetStatus(ctx context.Context, id, owner bson.ObjectID, status string, closesAt time.Time) (*Poll, error) {
	var p Poll
	err := s.polls.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "ownerId": owner},
		bson.M{"$set": bson.M{"status": status, "closesAt": closesAt, "updatedAt": time.Now().UTC()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &p, err
}

// CloseIfOpen is used by the auto-closer; it never reopens or touches closed polls.
func (s *Store) CloseIfOpen(ctx context.Context, id bson.ObjectID) (*Poll, error) {
	var p Poll
	err := s.polls.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "status": StatusOpen},
		bson.M{"$set": bson.M{"status": StatusClosed, "updatedAt": time.Now().UTC()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &p, err
}

// Delete removes an owned poll and its votes.
func (s *Store) Delete(ctx context.Context, id, owner bson.ObjectID) error {
	res, err := s.polls.DeleteOne(ctx, bson.M{"_id": id, "ownerId": owner})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	_, err = s.votes.DeleteMany(ctx, bson.M{"pollId": id})
	return err
}
