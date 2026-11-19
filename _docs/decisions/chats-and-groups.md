# ECHO decisions: chats, groups and messages

Up: [decisions](./README.md)

## Who may manage invite codes (#30)

Decided on 2026-11-15. Only the group's `owner` and `admin`s may create an invite code,
send one to a user, delete one, or list them. The list shows every code, with deleted and
expired ones marked. Everyone else in the group, pending members included, is refused.
`settings.allowInvites` doesn't widen this.

## How the message length limit counts (#51)

Decided on 2026-11-17. `content` may hold 4000 characters, counted as Unicode code points,
not bytes, so the limit doesn't shrink for non-Latin text. 4000 code points are at most
16000 bytes, which fits the 64 KiB frame limit (#40) even fully JSON-escaped.

## How a pending member accepts (#103, #5)

Decided on 2026-11-16. For now a `pending` member accepts by joining with any usable code
of the group (`POST groups/join/:inviteCode`), and owners and admins may `send` them one.
A dedicated accept and decline action, tied to the person's own invitation, is built with
#5 rather than added to the join route.

## Where a pending group invite is stored (#5)

Decided on 2026-11-18. A pending invite is the group's entry in the invitee's
`users.chats`, written when they are added, so `GET groups/pending` reads chats by `_id`
instead of scanning on the dynamic `participants.<id>` key. The hub skips a pending chat
until it is accepted. Accept and decline are REST only for now. Members left pending
before this, who were never asked and had full access, are accepted by a startup backfill,
so they keep that access. "Before" is a cutoff stored in `migrations` on the first run (the
start of the first server with this code), not a missing chats entry: a new invitee's
entry can be missing too, and they must never be accepted for them. Such an invitee gets
the entry instead. Accept-by-code
(#103) still works and finishes the same way.

## Where chat membership is stored

Decided on 2026-11-15. A chat's `participants` is the only record of who is in it.
`users.chats` isn't trusted for membership; it is dropped, or rebuilt from `participants`.
`GetUserChats` already checks the list against `participants`. Unblocks #14 and #17.

## Who counts as a chat member

Decided on 2026-11-07.

| Participant | Counts as a member? | In the code today |
|---|---|---|
| Pending | No | **Not enforced yet:** pending members still pass every membership check |
| Blocked | Yes | Already true. `send_message` still refuses them, as a separate check |
| Removed | No | Already true: removal deletes the participant entry |

Enforcing the pending rule has to wait for #5. Everyone added to a group is `pending`,
except the creator and people who join by invite code, and nobody can accept yet, so
enforcing it now would lock those members out. The order is: build accept and decline,
backfill existing members, then enforce it in `chatMemberFilter`. Direct-chat participants
have no status, so the check must allow for that.

## How a group's participant count is kept (#110)

Decided on 2026-11-19: `stats.participantCount` isn't stored any more. Reads compute it
from the size of `participants` (`participantCountExpr`; `GET groups/pending`, and the
create reply sets it in Go), so a write that forgets a counter, or an old server copy
during a deploy, can't leave it wrong. The `$inc` writes of #16 and the startup
`BackfillParticipantCounts` are gone. Groups saved before keep a stored value that
nothing reads. The size limit (#68) already counts `participants` itself.
