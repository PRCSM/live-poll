package store

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSameKeys(t *testing.T) {
	a := bson.D{{Key: "pollId", Value: 1}, {Key: "voterKey", Value: 1}}
	if !sameKeys(a, bson.D{{Key: "pollId", Value: int32(1)}, {Key: "voterKey", Value: int32(1)}}) {
		t.Error("int and int32 keys should match")
	}
	if sameKeys(a, bson.D{{Key: "voterKey", Value: 1}, {Key: "pollId", Value: 1}}) {
		t.Error("key order matters")
	}
	if sameKeys(bson.D{{Key: "_id", Value: -1}}, bson.D{{Key: "_id", Value: 1}}) {
		t.Error("direction matters")
	}
}
