package services

import "errors"

// Sentinel errors for expected domain conditions.
// Controllers map these to HTTP status codes.
var (
	ErrUserNotFound           = errors.New("user not found")
	ErrChatNotFound           = errors.New("chat not found")
	ErrMessageNotFound        = errors.New("message not found")
	ErrContactNotFound        = errors.New("contact not found")
	ErrContactRequestNotFound = errors.New("contact request not found")
	ErrInvalidInvite          = errors.New("invalid invite")
	ErrNotInContacts          = errors.New("user is not in your contacts")
	ErrNotInFavorites         = errors.New("user is not in your favorites")
	ErrAlreadyInContacts      = errors.New("user is already in your contacts")
	ErrAlreadyInFavorites     = errors.New("user is already in your favorites")
	ErrBlocked                = errors.New("user is blocked")
	ErrAlreadyBlocked         = errors.New("user is already blocked")
	ErrAlreadyUnblocked       = errors.New("user is already unblocked")
	ErrSelfBlock              = errors.New("cannot block yourself")
	ErrSelfContactRequest     = errors.New("cannot send a contact request to yourself")
	ErrSelfDirectChat         = errors.New("cannot open a direct chat with yourself")
	ErrRequestAlreadySent     = errors.New("contact request already sent")
	ErrNoValidParticipants    = errors.New("no valid participants to remove")
	ErrNotParticipant         = errors.New("you are not a participant of this chat")
	ErrPermissionDenied       = errors.New("permission denied")
	ErrAlreadyParticipant     = errors.New("users are already in the chat")
	ErrInvalidEmoji           = errors.New("invalid emoji")
	ErrAccountBanned          = errors.New("account is banned")
	ErrAccountInactive        = errors.New("account is not active")
	ErrTokenRevoked           = errors.New("token has been revoked")
	ErrTokenExpired           = errors.New("token has expired")
)
