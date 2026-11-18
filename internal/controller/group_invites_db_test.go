package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// callAsUser runs handler as userID, with groupID as the :groupID parameter when set.
func callAsUser(handler gin.HandlerFunc, method string, userID bson.ObjectID, groupID string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", nil)
	if groupID != "" {
		c.Params = gin.Params{{Key: "groupID", Value: groupID}}
	}
	c.Set("user", models.LoginUserResponse{ID: userID})
	handler(c)
	return w
}

// insertPendingGroup stores a group owned by owner with invitee pending, and the
// invitee's chats entry for it, as creating the group does (#5).
func insertPendingGroup(t *testing.T, owner, invitee bson.ObjectID) bson.ObjectID {
	t.Helper()
	ctx := t.Context()
	groupID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id": groupID, "chatType": string(constants.ChatTypeGroup), "name": "team",
		"participants": bson.M{
			owner.Hex():   bson.M{"role": string(constants.ChatRoleOwner), "requestStatus": string(constants.StatusAccepted)},
			invitee.Hex(): bson.M{"role": string(constants.ChatRoleMember), "requestStatus": string(constants.StatusPending), "requestedBy": owner},
		},
		"stats": bson.M{"participantCount": 2},
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	for _, id := range []bson.ObjectID{owner, invitee} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{
			"_id": id, "username": id.Hex(), "chats": bson.M{groupID.Hex(): 0},
		}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	return groupID
}

// The invitee lists, then accepts, a pending invite over REST (#5).
func TestGroupInvitePendingAndAcceptHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)
	owner, invitee := bson.NewObjectID(), bson.NewObjectID()
	groupID := insertPendingGroup(t, owner, invitee)

	w := callAsUser(GetPendingGroupInvitesHTTP, http.MethodGet, invitee, "")
	if w.Code != http.StatusOK {
		t.Fatalf("pending: status %d (body %s)", w.Code, w.Body)
	}
	var resp struct {
		Data []struct {
			ChatID string `json:"chatId"`
			Name   string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ChatID != groupID.Hex() || resp.Data[0].Name != "team" {
		t.Fatalf("pending body = %s, want the one group", w.Body)
	}

	if w := callAsUser(AcceptGroupInviteHTTP, http.MethodPost, invitee, groupID.Hex()); w.Code != http.StatusOK {
		t.Fatalf("accept: status %d (body %s)", w.Code, w.Body)
	}
	if w := callAsUser(AcceptGroupInviteHTTP, http.MethodPost, invitee, groupID.Hex()); w.Code != http.StatusConflict {
		t.Fatalf("accept again: status %d, want 409", w.Code)
	}
	if w := callAsUser(GetPendingGroupInvitesHTTP, http.MethodGet, invitee, ""); w.Code != http.StatusOK || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("pending after accept: status %d", w.Code)
	} else if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || len(resp.Data) != 0 {
		t.Fatalf("pending after accept = %s, want an empty list", w.Body)
	}
}

// Declining answers 200 once, then 404; an outsider and a bad ID get 404 and 400 (#5, #59).
func TestGroupInviteDeclineHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)
	owner, invitee, outsider := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	groupID := insertPendingGroup(t, owner, invitee)

	if w := callAsUser(DeclineGroupInviteHTTP, http.MethodPost, outsider, groupID.Hex()); w.Code != http.StatusNotFound {
		t.Fatalf("outsider: status %d, want 404", w.Code)
	}
	if w := callAsUser(DeclineGroupInviteHTTP, http.MethodPost, invitee, "not-hex"); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: status %d, want 400", w.Code)
	}
	if w := callAsUser(DeclineGroupInviteHTTP, http.MethodPost, invitee, groupID.Hex()); w.Code != http.StatusOK {
		t.Fatalf("decline: status %d (body %s)", w.Code, w.Body)
	}
	if w := callAsUser(DeclineGroupInviteHTTP, http.MethodPost, invitee, groupID.Hex()); w.Code != http.StatusNotFound {
		t.Fatalf("decline again: status %d, want 404", w.Code)
	}
}
