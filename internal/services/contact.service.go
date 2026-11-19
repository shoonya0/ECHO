package services

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ---------------------------------------------------------------------------
// Contact-list projection fields per status
// ---------------------------------------------------------------------------

var contactFieldsByStatus = map[constants.ContactStatus]struct {
	projection bson.M
	ids        func(models.ContactInfoEmbed) []bson.ObjectID
}{
	constants.StatusAccepted: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.contacts": 1, "contactInfo.favorites": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.Contacts },
	},
	constants.StatusPendingIn: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.pendingIn": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.PendingIn },
	},
	constants.StatusPendingOut: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.pendingOut": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.PendingOut },
	},
	constants.StatusFavorite: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.favorites": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.Favorites },
	},
	constants.StatusBlocked: {
		projection: bson.M{"_id": 1, "updatedAt": 1, "contactInfo.blocked": 1},
		ids:        func(c models.ContactInfoEmbed) []bson.ObjectID { return c.Blocked },
	},
	constants.StatusContact: {
		projection: bson.M{
			"_id":                    1,
			"updatedAt":              1,
			"contactInfo.contacts":   1,
			"contactInfo.favorites":  1,
			"contactInfo.pendingIn":  1,
			"contactInfo.pendingOut": 1,
		},
		// People you blocked are left out: GET /blocked lists them (#62).
		ids: func(c models.ContactInfoEmbed) []bson.ObjectID {
			return slices.Concat(c.PendingOut, c.PendingIn, c.Favorites, c.Contacts)
		},
	},
}

// userProjection is the standard fields returned when listing contacts.
var userProjection = bson.M{
	"_id":                   1,
	"profile":               1,
	"username":              1,
	"presence.status":       1,
	"presence.isOnline":     1,
	"presence.lastActivity": 1,
	"presence.lastSeen":     1,
	"settings.privacy":      1, // what the owner may see of their presence (#32)
}

// ============ CONTACTS & FRIENDS MANAGEMENT ============

// GetContacts returns one page of the user's list for contactStatus, in
// username order ignoring case, and the number of entries in the whole list.
// page counts from 1 to MaxPage, and limit must be at least 1 (#39). IDs
// whose user no longer exists are neither returned nor counted.
func GetContacts(ctx context.Context, userID bson.ObjectID, contactStatus constants.ContactStatus, page, limit int) ([]models.ContactInfo, int, error) {
	if page < 1 || page > MaxPage || limit < 1 {
		return []models.ContactInfo{}, 0, fmt.Errorf("invalid page %d or limit %d", page, limit)
	}

	cfg, ok := contactFieldsByStatus[contactStatus]
	if !ok {
		return []models.ContactInfo{}, 0, fmt.Errorf("unknown contact status: %s", contactStatus)
	}

	// The owner's accepted contacts decide what each listed user's
	// showOnlineStatus "contacts" lets them see (#32). Contacts are mutual.
	ownerProjection := maps.Clone(cfg.projection)
	ownerProjection["contactInfo.contacts"] = 1
	var contact models.ContactRequest
	opts := options.FindOne().SetProjection(ownerProjection)
	err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": userID}, opts).Decode(&contact)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.ContactInfo{}, 0, nil
		}
		return []models.ContactInfo{}, 0, err
	}

	contactIDs := cfg.ids(contact.ContactInfo)
	if len(contactIDs) == 0 {
		return []models.ContactInfo{}, 0, nil
	}

	users := database.DB.Collection(string(constants.UserColl))
	filter := bson.M{"_id": bson.M{"$in": contactIDs}}
	total, err := users.CountDocuments(ctx, filter)
	if err != nil {
		return []models.ContactInfo{}, 0, err
	}

	findOpts := options.Find().
		SetProjection(userProjection).
		SetSort(bson.D{{Key: "username", Value: 1}, {Key: "_id", Value: 1}}).
		SetCollation(&options.Collation{Locale: "en", Strength: 2}). // "Bob" between "alice" and "carol"
		SetSkip(int64(page-1) * int64(limit)).
		SetLimit(int64(limit))
	cursor, err := users.Find(ctx, filter, findOpts)
	if err != nil {
		return []models.ContactInfo{}, 0, err
	}
	defer cursor.Close(ctx)

	results := make([]models.ContactInfo, 0, min(limit, len(contactIDs)))
	privacy := make([]models.PrivacySettingsEmbed, 0, cap(results))
	for cursor.Next(ctx) {
		var listed struct {
			models.ContactInfo `bson:",inline"`
			Settings           struct {
				Privacy models.PrivacySettingsEmbed `bson:"privacy"`
			} `bson:"settings"`
		}
		if err := cursor.Decode(&listed); err != nil {
			return []models.ContactInfo{}, 0, err
		}
		user := listed.ContactInfo
		privacy = append(privacy, listed.Settings.Privacy)

		user.IsFavorite = contains(contact.ContactInfo.Favorites, user.ID)

		results = append(results, user)
	}

	if err := cursor.Err(); err != nil {
		return []models.ContactInfo{}, 0, err
	}

	// Presence comes from who is connected now, not the stored isOnline (#31).
	pageIDs := make([]bson.ObjectID, len(results))
	for i := range results {
		pageIDs[i] = results[i].ID
	}
	connected := connectedOrNone(ctx, pageIDs)
	for i := range results {
		viewer := ViewerOther
		if contains(contact.ContactInfo.Contacts, results[i].ID) {
			viewer = ViewerContact
		}
		ShowPresence(&results[i].Presence, privacy[i], connected[results[i].ID], viewer)
	}
	return results, int(total), nil
}

// ============= Contact Actions =============

// userContactProjection returns standard projection for contact-validation reads.
func userContactProjection() bson.M {
	return bson.M{
		"_id":                    1,
		"contactInfo.blocked":    1,
		"contactInfo.blockedBy":  1,
		"contactInfo.pendingOut": 1,
		"contactInfo.pendingIn":  1,
		"contactInfo.favorites":  1,
		"contactInfo.contacts":   1,
		// Who may send them a request (#32).
		"settings.privacy.allowContactBy": 1,
	}
}

// fetchUserContact fetches a user with contact-related projection.
func fetchUserContact(ctx context.Context, userID bson.ObjectID) (*models.User, error) {
	var user models.User
	filter := bson.M{"_id": userID}
	opts := options.FindOne().SetProjection(userContactProjection())
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, filter, opts).Decode(&user); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("%w: %w", ErrUserNotFound, err)
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	return &user, nil
}

// checkContactRequest validates a contact request between two users.
func checkContactRequest(user *models.User, target bson.ObjectID) error {
	switch {
	case contains(user.ContactInfo.PendingOut, target):
		return fmt.Errorf("%w", ErrRequestAlreadySent)
	case contains(user.ContactInfo.PendingIn, target):
		return fmt.Errorf("%w", ErrRequestAlreadySent)
	case contains(user.ContactInfo.Favorites, target):
		return fmt.Errorf("%w", ErrAlreadyInFavorites)
	case contains(user.ContactInfo.Contacts, target):
		return fmt.Errorf("%w", ErrAlreadyInContacts)
	case isBlockedEitherWay(user, target):
		return fmt.Errorf("%w", ErrBlocked)
	}
	return nil
}

// checkAllowContact applies target's settings.privacy.allowContactBy to a
// request from from (#32). "contacts" lets in people who share a group with
// the target: as no one is a contact before a request, the people the target
// already knows. A value the server doesn't know lets nobody in.
func checkAllowContact(ctx context.Context, from bson.ObjectID, target *models.User) error {
	switch constants.PrivacyScope(target.Settings.Privacy.AllowContactBy) {
	case "", constants.PrivacyEveryone:
		return nil
	case constants.PrivacyContacts:
		shared, err := sharesGroup(ctx, from, target.ID)
		if err != nil {
			return err
		}
		if shared {
			return nil
		}
	}
	return fmt.Errorf("%w", ErrContactNotAllowed)
}

// sharesGroup reports whether a and b are both accepted members of one group. Direct chats don't count: anyone can open one with
// anyone, so they would let everyone in.
func sharesGroup(ctx context.Context, a, b bson.ObjectID) (bool, error) {
	cur, err := database.DB.Collection(string(constants.UserColl)).Find(ctx,
		bson.M{"_id": bson.M{"$in": bson.A{a, b}}},
		options.Find().SetProjection(bson.M{"chats": 1}))
	if err != nil {
		return false, fmt.Errorf("read chats: %w", err)
	}
	var both []models.User
	if err := cur.All(ctx, &both); err != nil {
		return false, fmt.Errorf("read chats: %w", err)
	}
	if len(both) != 2 {
		return false, nil
	}
	var common []bson.ObjectID
	for chatID := range both[0].Chats {
		if _, ok := both[1].Chats[chatID]; ok {
			common = append(common, chatID)
		}
	}
	if len(common) == 0 {
		return false, nil
	}
	// Accepted members only: any other status, known or not, doesn't count.
	filter := bson.M{"_id": bson.M{"$in": common}, "chatType": string(constants.ChatTypeGroup)}
	for _, id := range []bson.ObjectID{a, b} {
		filter["participants."+id.Hex()+".requestStatus"] = string(constants.StatusAccepted)
	}
	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, filter, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("find a shared group: %w", err)
	}
	return n > 0, nil
}

// SendContactRequest sends a contact request from userID to targetUserID.
//
// The checks and both writes run in one transaction (#37). A request the other
// way sent at the same moment writes the same two documents, so one of the two
// commits and the other runs again, sees it and gets ErrRequestAlreadySent.
func SendContactRequest(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	if userID == targetUserID {
		return fmt.Errorf("%w", ErrSelfContactRequest) // #38
	}
	return database.WithTransaction(ctx, func(ctx context.Context) error {
		return sendContactRequest(ctx, userID, targetUserID)
	})
}

func sendContactRequest(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	targetUser, err := fetchUserContact(ctx, targetUserID)
	if err != nil {
		return err
	}

	if err := checkContactRequest(targetUser, userID); err != nil {
		return err
	}
	if err := checkContactRequest(user, targetUserID); err != nil {
		return err
	}
	if err := checkAllowContact(ctx, userID, targetUser); err != nil {
		return err
	}

	now := time.Now()
	coll := database.DB.Collection(string(constants.UserColl))

	// A concurrent request (a double tap, or the other way) that commits after
	// this transaction's reads makes these writes conflict; the transaction then
	// runs again and checkContactRequest returns the usual error (#37).
	return writeInIDOrder(userID, targetUserID, func() error {
		sent, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
			"$addToSet": bson.M{"contactInfo.pendingOut": targetUserID},
			"$set":      bson.M{"contactInfo.updatedAt": now},
		})
		if err != nil {
			return fmt.Errorf("failed to update requesting user: %w", err)
		}
		if sent.MatchedCount == 0 {
			return fmt.Errorf("%w", ErrUserNotFound)
		}
		return nil
	}, func() error {
		received, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, bson.M{
			"$addToSet": bson.M{"contactInfo.pendingIn": userID},
			"$set":      bson.M{"contactInfo.updatedAt": now},
		})
		if err != nil {
			return fmt.Errorf("failed to update target user: %w", err)
		}
		if received.MatchedCount == 0 {
			return fmt.Errorf("%w", ErrUserNotFound) // deleted since the check
		}
		return nil
	})
}

// AcceptOrDeclineContactRequest accepts or declines a pending contact request.
// Both sides change in one transaction (#37), so neither keeps a one-sided contact.
func AcceptOrDeclineContactRequest(ctx context.Context, userID, targetRequestID bson.ObjectID, action string) error {
	return database.WithTransaction(ctx, func(ctx context.Context) error {
		return answerContactRequest(ctx, userID, targetRequestID, action)
	})
}

func answerContactRequest(ctx context.Context, userID, targetRequestID bson.ObjectID, action string) error {
	// The writes are conditional on the request being there on both sides; one
	// that matches nothing ends the transaction with ErrContactRequestNotFound.
	userFilter := bson.M{"_id": userID, "contactInfo.pendingIn": bson.M{"$in": []bson.ObjectID{targetRequestID}}}
	targetFilter := bson.M{"_id": targetRequestID, "contactInfo.pendingOut": bson.M{"$in": []bson.ObjectID{userID}}}

	coll := database.DB.Collection(string(constants.UserColl))

	now := time.Now()
	var userUpdate, targetUpdate bson.M

	switch constants.ContactStatus(action) {
	case constants.StatusAccepted:
		userUpdate = bson.M{
			"$pull":     bson.M{"contactInfo.pendingIn": targetRequestID},
			"$addToSet": bson.M{"contactInfo.contacts": targetRequestID},
			"$set":      bson.M{"contactInfo.updatedAt": now},
		}
		targetUpdate = bson.M{
			"$pull":     bson.M{"contactInfo.pendingOut": userID},
			"$addToSet": bson.M{"contactInfo.contacts": userID},
			"$set":      bson.M{"contactInfo.updatedAt": now},
		}
	case constants.StatusDeclined:
		userUpdate = bson.M{
			"$pull": bson.M{"contactInfo.pendingIn": targetRequestID},
		}
		targetUpdate = bson.M{
			"$pull": bson.M{"contactInfo.pendingOut": userID},
		}
	default:
		return fmt.Errorf("unknown action: %s (use 'accepted' or 'declined')", action)
	}

	return writeInIDOrder(userID, targetRequestID, func() error {
		res, err := UpdateOne(ctx, coll, userFilter, userUpdate)
		if err != nil {
			return fmt.Errorf("failed to update user contact: %w", err)
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("%w", ErrContactRequestNotFound)
		}
		return nil
	}, func() error {
		res, err := UpdateOne(ctx, coll, targetFilter, targetUpdate)
		if err != nil {
			return fmt.Errorf("failed to update target contact: %w", err)
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("%w", ErrContactRequestNotFound)
		}
		return nil
	})
}

// RemoveContact removes a contact relationship between two users, on both
// sides in one transaction (#37).
func RemoveContact(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	return database.WithTransaction(ctx, func(ctx context.Context) error {
		return removeContact(ctx, userID, targetUserID)
	})
}

func removeContact(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	if !contains(user.ContactInfo.Contacts, targetUserID) {
		return fmt.Errorf("%w", ErrNotInContacts)
	}

	now := time.Now()
	coll := database.DB.Collection(string(constants.UserColl))

	return writeInIDOrder(userID, targetUserID, func() error {
		res, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, bson.M{
			"$pull": bson.M{
				"contactInfo.favorites": targetUserID,
				"contactInfo.contacts":  targetUserID,
			},
			"$set": bson.M{"contactInfo.updatedAt": now},
		})
		if err != nil {
			return fmt.Errorf("failed to update user contact: %w", err)
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("%w: caller no longer exists", ErrUserNotFound)
		}
		return nil
	}, func() error {
		// MatchedCount isn't checked: a target whose account is gone matches
		// nothing, and the caller's side is still removed, or a contact left
		// over from a deleted account could never be removed.
		if _, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, bson.M{
			"$pull": bson.M{
				"contactInfo.favorites": userID,
				"contactInfo.contacts":  userID,
			},
			"$set": bson.M{"contactInfo.updatedAt": now},
		}); err != nil {
			return fmt.Errorf("failed to update target contact: %w", err)
		}
		return nil
	})
}

// BlockUnblockUser blocks or unblocks a user. A block is one-sided (#26): it goes in
// the caller's contactInfo.blocked and the target's hidden contactInfo.blockedBy, so
// only the caller can lift it. Blocking also removes the target from the
// contacts/favourites/pending lists on both sides.
//
// Both writes run in one transaction (#37). "Already blocked/unblocked" is still
// decided from both documents, so a half-written block from before the
// transactions is repaired by doing it again.
func BlockUnblockUser(ctx context.Context, userID, targetUserID bson.ObjectID, action string) error {
	if userID == targetUserID {
		return fmt.Errorf("%w", ErrSelfBlock)
	}
	return database.WithTransaction(ctx, func(ctx context.Context) error {
		return blockUnblockUser(ctx, userID, targetUserID, action)
	})
}

func blockUnblockUser(ctx context.Context, userID, targetUserID bson.ObjectID, action string) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}
	target, err := fetchUserContact(ctx, targetUserID)
	if err != nil {
		return err
	}
	// Both halves of the caller's own block. A block the target made is not looked at.
	callerHalf := contains(user.ContactInfo.Blocked, targetUserID)
	targetHalf := contains(target.ContactInfo.BlockedBy, userID)

	now := time.Now()
	coll := database.DB.Collection(string(constants.UserColl))
	var callerUpdate, targetUpdate bson.M

	switch constants.ContactStatus(action) {
	case constants.StatusBlocked:
		if callerHalf && targetHalf {
			return fmt.Errorf("%w", ErrAlreadyBlocked)
		}
		callerUpdate = bson.M{
			"$addToSet": bson.M{"contactInfo.blocked": targetUserID},
			"$pull":     relationshipLists(targetUserID),
			"$set":      bson.M{"contactInfo.updatedAt": now},
		}
		targetUpdate = bson.M{
			"$addToSet": bson.M{"contactInfo.blockedBy": userID},
			"$pull":     relationshipLists(userID),
			"$set":      bson.M{"contactInfo.updatedAt": now},
		}

	case constants.StatusUnblocked:
		if !callerHalf && !targetHalf {
			return fmt.Errorf("%w", ErrAlreadyUnblocked)
		}
		callerUpdate = bson.M{
			"$pull": bson.M{"contactInfo.blocked": targetUserID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}
		targetUpdate = bson.M{
			"$pull": bson.M{"contactInfo.blockedBy": userID},
			"$set":  bson.M{"contactInfo.updatedAt": now},
		}

	default:
		return fmt.Errorf("unknown action: %s (use 'blocked' or 'unblocked')", action)
	}

	return writeInIDOrder(userID, targetUserID, func() error {
		res, err := UpdateOne(ctx, coll, bson.M{"_id": userID}, callerUpdate)
		if err != nil {
			return fmt.Errorf("failed to %s user: %w", action, err)
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("%w: caller no longer exists", ErrUserNotFound)
		}
		return nil
	}, func() error {
		res, err := UpdateOne(ctx, coll, bson.M{"_id": targetUserID}, targetUpdate)
		if err != nil {
			return fmt.Errorf("failed to update %s user: %w", action, err)
		}
		if res.MatchedCount == 0 {
			return fmt.Errorf("%w: target no longer exists", ErrUserNotFound)
		}
		return nil
	})
}

// relationshipLists is the $pull that removes id from every contact list a block ends.
func relationshipLists(id bson.ObjectID) bson.M {
	return bson.M{
		"contactInfo.contacts":   id,
		"contactInfo.favorites":  id,
		"contactInfo.pendingIn":  id,
		"contactInfo.pendingOut": id,
	}
}

// isBlockedEitherWay reports whether user blocked target or target blocked user.
func isBlockedEitherWay(user *models.User, target bson.ObjectID) bool {
	return contains(user.ContactInfo.Blocked, target) || contains(user.ContactInfo.BlockedBy, target)
}

// IsBlockedBetween reports whether either user has blocked the other (#27). It reads
// each side's own contactInfo.blocked, which BlockUnblockUser writes first, so a block
// whose second write (the other side's blockedBy) never landed still counts.
func IsBlockedBetween(ctx context.Context, a, b bson.ObjectID) (bool, error) {
	filter := bson.M{"$or": []bson.M{
		{"_id": a, "contactInfo.blocked": b},
		{"_id": b, "contactInfo.blocked": a},
	}}
	opts := options.FindOne().SetProjection(bson.M{"_id": 1})
	err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, filter, opts).Err()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check blocks: %w", err)
	}
	return true, nil
}

// AddToFavorites adds a user to the current user's favorites.
func AddToFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	if !contains(user.ContactInfo.Contacts, targetUserID) {
		return fmt.Errorf("%w", ErrNotInContacts)
	}
	if isBlockedEitherWay(user, targetUserID) {
		return fmt.Errorf("%w", ErrBlocked)
	}
	if contains(user.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("%w", ErrAlreadyInFavorites)
	}

	now := time.Now()
	// Conditional on the checks above still holding, so a contact removed in
	// between isn't favorited, and of two concurrent adds one gets
	// ErrAlreadyInFavorites (#37).
	res, err := UpdateOne(ctx, database.DB.Collection(string(constants.UserColl)), bson.M{
		"_id":                   userID,
		"contactInfo.contacts":  targetUserID,
		"contactInfo.favorites": bson.M{"$ne": targetUserID},
	}, bson.M{
		"$addToSet": bson.M{"contactInfo.favorites": targetUserID},
		"$set":      bson.M{"contactInfo.updatedAt": now},
	})
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}
	if res.MatchedCount == 0 {
		// Read again to say which check failed.
		user, err := fetchUserContact(ctx, userID)
		if err != nil {
			return err
		}
		if contains(user.ContactInfo.Favorites, targetUserID) {
			return fmt.Errorf("%w", ErrAlreadyInFavorites)
		}
		return fmt.Errorf("%w", ErrNotInContacts)
	}

	return nil
}

// RemoveFromFavorites removes a user from the current user's favorites.
func RemoveFromFavorites(ctx context.Context, userID, targetUserID bson.ObjectID) error {
	user, err := fetchUserContact(ctx, userID)
	if err != nil {
		return err
	}

	if !contains(user.ContactInfo.Favorites, targetUserID) {
		return fmt.Errorf("%w", ErrNotInFavorites)
	}

	now := time.Now()
	_, err = UpdateOne(ctx, database.DB.Collection(string(constants.UserColl)), bson.M{"_id": userID}, bson.M{
		"$pull": bson.M{"contactInfo.favorites": targetUserID},
		"$set":  bson.M{"contactInfo.updatedAt": now},
	})
	if err != nil {
		return fmt.Errorf("failed to update user contact: %w", err)
	}

	return nil
}
