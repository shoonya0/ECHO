package services

import (
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// RemoveGroupMember rules (#28): an admin can't remove the owner, and the
// actor's own chats entry stays when they list themselves among the targets.
func TestRemoveGroupMemberRules(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	chats := database.DB.Collection(string(constants.ChatColl))

	owner, admin, admin2, member, member2 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()
	all := []bson.ObjectID{owner, admin, admin2, member, member2}

	for _, id := range all {
		if _, err := users.InsertOne(ctx, bson.M{"_id": id, "username": id.Hex(), "chats": bson.M{chatID.Hex(): 0}}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	role := func(r constants.ChatRole) bson.M {
		return bson.M{"role": string(r), "requestStatus": "accepted"}
	}
	if _, err := chats.InsertOne(ctx, bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"participants": bson.M{
			owner.Hex():   role(constants.ChatRoleOwner),
			admin.Hex():   role(constants.ChatRoleAdmin),
			admin2.Hex():  role(constants.ChatRoleAdmin),
			member.Hex():  role(constants.ChatRoleMember),
			member2.Hex(): role(constants.ChatRoleMember),
		},
		"stats": bson.M{"participantCount": len(all)},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	inGroup := func(t *testing.T, id bson.ObjectID) bool {
		t.Helper()
		var chat models.Chat
		if err := chats.FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat); err != nil {
			t.Fatalf("load chat: %v", err)
		}
		_, ok := chat.Participants[id]
		return ok
	}
	hasChatRef := func(t *testing.T, id bson.ObjectID) bool {
		t.Helper()
		n, err := users.CountDocuments(ctx, bson.M{"_id": id, "chats." + chatID.Hex(): bson.M{"$exists": true}})
		if err != nil {
			t.Fatalf("count user: %v", err)
		}
		return n == 1
	}

	t.Run("admin cannot remove the owner", func(t *testing.T) {
		err := RemoveGroupMember(ctx, admin, chatID, []bson.ObjectID{owner})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
		if !inGroup(t, owner) || !hasChatRef(t, owner) {
			t.Fatal("the owner was removed by an admin")
		}
	})

	t.Run("a request that includes the owner removes nobody", func(t *testing.T) {
		err := RemoveGroupMember(ctx, admin, chatID, []bson.ObjectID{member2, owner})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
		if !inGroup(t, member2) || !hasChatRef(t, member2) {
			t.Fatal("member2 was removed by a refused request")
		}
	})

	t.Run("listing yourself keeps your own chat entry", func(t *testing.T) {
		if err := RemoveGroupMember(ctx, admin, chatID, []bson.ObjectID{admin, member}); err != nil {
			t.Fatalf("RemoveGroupMember: %v", err)
		}
		if inGroup(t, member) || hasChatRef(t, member) {
			t.Fatal("member is still in the group")
		}
		if !inGroup(t, admin) || !hasChatRef(t, admin) {
			t.Fatal("the admin lost their own place or chats entry")
		}
	})

	t.Run("owner removes an admin", func(t *testing.T) {
		if err := RemoveGroupMember(ctx, owner, chatID, []bson.ObjectID{admin2}); err != nil {
			t.Fatalf("RemoveGroupMember: %v", err)
		}
		if inGroup(t, admin2) || hasChatRef(t, admin2) {
			t.Fatal("admin2 is still in the group")
		}
	})

	t.Run("member cannot remove anyone", func(t *testing.T) {
		err := RemoveGroupMember(ctx, member2, chatID, []bson.ObjectID{admin})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
	})
}
