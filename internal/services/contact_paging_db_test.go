package services

import (
	"slices"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #39: the contact lists are cut to limit, one page at a time, in username
// order, and total counts the entries that exist.
func TestGetContactsPages(t *testing.T) {
	ctx := useTestDB(t)

	owner := bson.NewObjectID()
	names := []string{"erin", "dave", "carol", "bob", "alice"}
	ids := make(map[string]bson.ObjectID, len(names))
	docs := []any{}
	contactIDs := bson.A{bson.NewObjectID()} // a deleted user: listed, but no document
	for _, name := range names {
		ids[name] = bson.NewObjectID()
		contactIDs = append(contactIDs, ids[name])
		docs = append(docs, bson.M{"_id": ids[name], "username": name})
	}
	docs = append(docs, bson.M{"_id": owner, "username": "owner", "contactInfo": bson.M{"contacts": contactIDs}})
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert users: %v", err)
	}

	cases := []struct {
		page, limit int
		want        []string
	}{
		{1, 2, []string{"alice", "bob"}},
		{2, 2, []string{"carol", "dave"}},
		{3, 2, []string{"erin"}},
		{4, 2, []string{}},
		{1, 10, []string{"alice", "bob", "carol", "dave", "erin"}},
	}
	for _, tc := range cases {
		list, total, err := GetContacts(ctx, owner, constants.StatusAccepted, tc.page, tc.limit)
		if err != nil {
			t.Fatalf("page %d limit %d: %v", tc.page, tc.limit, err)
		}
		got := make([]string, 0, len(list))
		for _, u := range list {
			got = append(got, u.Username)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("page %d limit %d: got %v, want %v", tc.page, tc.limit, got, tc.want)
		}
		if total != len(names) {
			t.Errorf("page %d limit %d: total = %d, want %d (the deleted user isn't counted)", tc.page, tc.limit, total, len(names))
		}
	}
}
