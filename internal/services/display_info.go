package services

import (
	"context"
	"fmt"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DisplayInfo is the name and avatar shown for an account.
type DisplayInfo struct {
	Username    string
	DisplayName string
	Avatar      string
}

// LookupDisplayInfo reads the current name and avatar of each account, in one query.
// Replies use it instead of the copies stored in chats and messages, which kept the
// name from when they were written (#36). An account that no longer exists is shown
// as DeletedUserName, like the copies account deletion rewrites (#35).
func LookupDisplayInfo(ctx context.Context, ids []bson.ObjectID) (map[bson.ObjectID]DisplayInfo, error) {
	out := make(map[bson.ObjectID]DisplayInfo, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := database.DB.Collection(string(constants.UserColl)).Find(ctx,
		bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"username": 1, "profile.displayName": 1, "profile.avatar": 1}))
	if err != nil {
		return nil, fmt.Errorf("failed to read display info: %w", err)
	}
	var users []struct {
		ID       bson.ObjectID `bson:"_id"`
		Username string        `bson:"username"`
		Profile  struct {
			DisplayName string `bson:"displayName"`
			Avatar      string `bson:"avatar"`
		} `bson:"profile"`
	}
	if err := cur.All(ctx, &users); err != nil {
		return nil, fmt.Errorf("failed to decode display info: %w", err)
	}
	for _, u := range users {
		out[u.ID] = DisplayInfo{Username: u.Username, DisplayName: u.Profile.DisplayName, Avatar: u.Profile.Avatar}
	}
	for _, id := range ids {
		if _, ok := out[id]; !ok {
			out[id] = DisplayInfo{DisplayName: DeletedUserName}
		}
	}
	return out, nil
}

// fillMessageNames replaces the names copied into each message (the sender, the
// usernames in reactions' details and in mentions) with the current ones, read in
// one query for the page (#36).
func fillMessageNames(ctx context.Context, messages []models.Message) error {
	seen := make(map[bson.ObjectID]bool)
	var ids []bson.ObjectID
	add := func(id bson.ObjectID) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, m := range messages {
		add(m.SenderID)
		for _, r := range m.Reactions {
			for _, id := range r.Users {
				add(id)
			}
		}
		for _, id := range m.Mentions.UserIDs {
			add(id)
		}
	}
	info, err := LookupDisplayInfo(ctx, ids)
	if err != nil {
		return err
	}
	for i := range messages {
		m := &messages[i]
		d := info[m.SenderID]
		m.Sender = models.MessageSenderEmbed{UserID: m.SenderID, Username: d.Username, DisplayName: d.DisplayName, Avatar: d.Avatar}
		for emoji, r := range m.Reactions {
			r.Details = make(map[string]string, len(r.Users))
			for _, id := range r.Users {
				r.Details[id.Hex()] = info[id].Username
			}
			m.Reactions[emoji] = r
		}
		if len(m.Mentions.UserIDs) > 0 {
			m.Mentions.UserNames = make([]string, len(m.Mentions.UserIDs))
			for j, id := range m.Mentions.UserIDs {
				m.Mentions.UserNames[j] = info[id].Username
			}
		}
	}
	return nil
}

// FillParticipantInfo replaces each participant's stored userInfo with the current
// name and avatar (#36), for a chat about to be sent to a client.
func FillParticipantInfo(ctx context.Context, chat *models.Chat) error {
	if chat == nil || len(chat.Participants) == 0 {
		return nil
	}
	ids := make([]bson.ObjectID, 0, len(chat.Participants))
	for id := range chat.Participants {
		ids = append(ids, id)
	}
	info, err := LookupDisplayInfo(ctx, ids)
	if err != nil {
		return err
	}
	for id, p := range chat.Participants {
		d := info[id]
		p.UserInfo = models.ContactUserInfo{UserID: id, Username: d.Username, DisplayName: d.DisplayName, Avatar: d.Avatar}
		chat.Participants[id] = p
	}
	return nil
}

// WithCurrentDisplayInfo returns the request's user with the name and avatar read
// from the database. The request's user is built from the JWT, which keeps the
// name from login for 7 days and has no avatar, so copies written from it were
// stale or empty (#36).
func WithCurrentDisplayInfo(ctx context.Context, user models.LoginUserResponse) (models.LoginUserResponse, error) {
	info, err := LookupDisplayInfo(ctx, []bson.ObjectID{user.ID})
	if err != nil {
		return user, err
	}
	d := info[user.ID]
	user.Username, user.Profile.DisplayName, user.Profile.Avatar = d.Username, d.DisplayName, d.Avatar
	return user, nil
}
