package auth

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type User struct {
	ID           bson.ObjectID `bson:"_id" json:"id"`
	Email        string        `bson:"email" json:"email"`
	Name         string        `bson:"name" json:"name"`
	PasswordHash string        `bson:"passwordHash" json:"-"`
	TokenVersion int           `bson:"tokenVersion" json:"-"`
	CreatedAt    time.Time     `bson:"createdAt" json:"createdAt"`
}

var (
	errEmailTaken = errors.New("email taken")
	errNoUser     = errors.New("no such user")
)

type Users struct{ coll *mongo.Collection }

func NewUsers(db *mongo.Database) *Users { return &Users{coll: db.Collection("users")} }

func (s *Users) Create(ctx context.Context, u *User) error {
	_, err := s.coll.InsertOne(ctx, u)
	if mongo.IsDuplicateKeyError(err) {
		return errEmailTaken
	}
	return err
}

func (s *Users) ByEmail(ctx context.Context, email string) (*User, error) {
	return s.findOne(ctx, bson.M{"email": email})
}

func (s *Users) ByID(ctx context.Context, id bson.ObjectID) (*User, error) {
	return s.findOne(ctx, bson.M{"_id": id})
}

func (s *Users) findOne(ctx context.Context, filter bson.M) (*User, error) {
	var u User
	err := s.coll.FindOne(ctx, filter).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, errNoUser
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// BumpTokenVersion invalidates every token issued to the user so far.
func (s *Users) BumpTokenVersion(ctx context.Context, id bson.ObjectID) error {
	_, err := s.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"tokenVersion": 1}})
	return err
}
