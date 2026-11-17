package services

import (
	"slices"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #39: the contact lists are cut to limit, one page at a time, in username
// order (ignoring case), and total counts the entries that exist.
func TestGetContactsPages(t *testing.T) {
	ctx := useTestDB(t)

	owner := bson.NewObjectID()
	names := []string{"erin", "Dave", "carol", "Bob", "alice"}
	ids := make(map[string]bson.ObjectID, len(names))
	docs := []any{}
	contactIDs := bson.A{bson.NewObjectID()} // a deleted user: listed, but no document
	for _, name := range names {
		ids[name] = bson.NewObjectID()
		contactIDs = append(contactIDs, ids[name])
		docs = append(docs, bson.M{"_id": ids[name], "username": name})
	}
	// erin is a favorite too, so the merged "contacts" list holds her ID twice.
	docs = append(docs, bson.M{"_id": owner, "username": "owner", "contactInfo": bson.M{
		"contacts":  contactIDs,
		"favorites": bson.A{ids["erin"]},
	}})
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert users: %v", err)
	}

	cases := []struct {
		page, limit int
		want        []string
	}{
		{1, 2, []string{"alice", "Bob"}},
		{2, 2, []string{"carol", "Dave"}},
		{3, 2, []string{"erin"}},
		{4, 2, []string{}},
		{1, 10, []string{"alice", "Bob", "carol", "Dave", "erin"}},
	}
	for _, status := range []constants.ContactStatus{constants.StatusAccepted, constants.StatusContact} {
		for _, tc := range cases {
			list, total, err := GetContacts(ctx, owner, status, tc.page, tc.limit)
			if err != nil {
				t.Fatalf("%s page %d limit %d: %v", status, tc.page, tc.limit, err)
			}
			got := make([]string, 0, len(list))
			for _, u := range list {
				got = append(got, u.Username)
				if u.IsFavorite != (u.Username == "erin") {
					t.Errorf("%s page %d: %s has isFavorite %v", status, tc.page, u.Username, u.IsFavorite)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s page %d limit %d: got %v, want %v", status, tc.page, tc.limit, got, tc.want)
			}
			if total != len(names) {
				t.Errorf("%s page %d limit %d: total = %d, want %d (each user once, the deleted one not at all)", status, tc.page, tc.limit, total, len(names))
			}
		}
	}

	// A page past MaxPage is refused instead of overflowing the skip.
	if _, _, err := GetContacts(ctx, owner, constants.StatusAccepted, MaxPage+1, 100); err == nil {
		t.Error("a page past MaxPage was accepted")
	}
}
