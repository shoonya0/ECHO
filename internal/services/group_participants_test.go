package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #18: the creator of a group must be stored as its owner, or nobody can add or remove
// members (AddGroupMember / RemoveGroupMember require owner or admin).
func TestNewGroupParticipants(t *testing.T) {
	creator := models.LoginUserResponse{ID: bson.NewObjectID(), Username: "creator"}
	alice := models.LoginUserResponse{ID: bson.NewObjectID(), Username: "alice"}
	bob := models.LoginUserResponse{ID: bson.NewObjectID(), Username: "bob"}

	t.Run("creator is accepted owner, invitees are pending members", func(t *testing.T) {
		p := newGroupParticipants(creator, []models.LoginUserResponse{alice, bob})

		if len(p) != 3 {
			t.Fatalf("len(participants) = %d, want 3", len(p))
		}
		if got := p[creator.ID].Role; got != string(constants.ChatRoleOwner) {
			t.Fatalf("creator role = %q, want %q", got, constants.ChatRoleOwner)
		}
		if got := p[creator.ID].RequestStatus; got != string(constants.StatusAccepted) {
			t.Fatalf("creator status = %q, want %q", got, constants.StatusAccepted)
		}
		for _, u := range []models.LoginUserResponse{alice, bob} {
			if got := p[u.ID].Role; got != string(constants.ChatRoleMember) {
				t.Fatalf("%s role = %q, want %q", u.Username, got, constants.ChatRoleMember)
			}
			if got := p[u.ID].RequestStatus; got != string(constants.StatusPending) {
				t.Fatalf("%s status = %q, want %q", u.Username, got, constants.StatusPending)
			}
		}
	})

	t.Run("creator listed as a participant stays owner", func(t *testing.T) {
		p := newGroupParticipants(creator, []models.LoginUserResponse{alice, creator})

		if len(p) != 2 {
			t.Fatalf("len(participants) = %d, want 2", len(p))
		}
		if got := p[creator.ID].Role; got != string(constants.ChatRoleOwner) {
			t.Fatalf("creator role = %q, want %q", got, constants.ChatRoleOwner)
		}
		if got := p[creator.ID].RequestStatus; got != string(constants.StatusAccepted) {
			t.Fatalf("creator status = %q, want %q", got, constants.StatusAccepted)
		}
	})
}
