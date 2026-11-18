# ECHO: decisions

Choices that decide how open issues get fixed. Each one says what was decided, when, and
what it means for the code. Questions still open are in
[current-state.md](./current-state.md#open-decisions).

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

## What deleting an account does

Decided on 2026-11-18, replacing the 2026-11-15 choice of a soft delete (#35). The account
document is deleted, and what points at it is cleaned first: the ID leaves every other
user's contact, favorite, request and block lists; groups drop the member; the name and
avatar copied into their messages and direct-chat entries become "Deleted user", so other
people's history stays whole. A soft delete would have needed a "not deleted" filter in
every one of the ~37 reads of other users, which already treat a missing account as gone
(the auth check answers `ACCOUNT_NOT_FOUND`), and IDs can't be reused (#34). A deleted owner
of a group or channel hands it to another owner, else the earliest-joined admin, else the
earliest-joined accepted member; a co-owner leaves the primary owner (`ownerId`) alone.
With nobody to hand it to, the group has no owner and its invite codes are deleted. The
account's sockets close and its token is locked before the cleanup, the account goes
last (a delete that stops partway can be repeated), and a second pass after it catches
anything others added meanwhile. Finding the chats and messages scans (no index on
`participants.<id>` or `senderId`), which is fine for a rare action.

## MongoDB transactions

Decided on 2026-11-15. Multi-document writes may use transactions, so MongoDB must run as a
replica set everywhere. Atlas already does. Local `docker compose` and CI run it as a
one-node replica set (`--replSet rs0`, initialized once). Unblocks #37.

## How blocks are stored

Decided on 2026-11-08 (#26). A block is stored on both people, with direction:
`contactInfo.blocked` on the blocker and `contactInfo.blockedBy` on the blocked person.
`blockedBy` is never sent to clients. Old `blockedChats` data has no direction, so the
startup migration (`MigrateLegacyBlocks`) turns each pair into a mutual block and removes
the old field. Nobody is unblocked without choosing to be.

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

## Changing the email (#36)

Decided on 2026-11-18. `PUT profile/` no longer changes `email`. It took any string, with
no password or format check, so a stolen token could move the account to another
address, and a taken email answered `500`. Emails are stored trimmed and in lower case.
Signup and login match them ignoring case (a collation index, `email_ci`), so emails
stored with capitals before #36 work without waiting for the startup migration that lowers
them. Two old accounts whose emails differ only in case are both kept: login picks the one
stored exactly as typed, and the migration logs them for a manual merge. A change-email
flow, with the current password and a check of the new address, is a separate issue.

## How login and signup limits find the client (#40)

Decided on 2026-11-17. Login attempts on an account are counted per IP and account (10
in 15 minutes) and per account from all IPs (100 an hour). The first stops one address from
guessing without locking the owner out; locking an account takes several addresses at that
limit. The second caps guessing spread over many addresses. Both are counted before bcrypt,
only for accounts that exist, and a successful login clears them. Logins and signups are
also counted per client IP.
The IP comes from the header named in `CLIENT_IP_HEADER` (`True-Client-IP` on Render, set
by Cloudflare in front of it), or else the socket address. `X-Forwarded-For` is never
trusted, because Render's proxy appends to what the client sent, and a header value that
isn't an IP is dropped. Per-IP limits are
generous because mobile carriers put many users behind one address. Counts live in Redis.
When Redis is down, requests go through: an outage shouldn't block every login.

Accepted trade-off: only existing accounts reach the per-account limits, so after 10 tries a
real email gets `429` and a made-up one keeps getting `401`. That shows which emails have
accounts, but `signup` already answers `409` for a taken email, and an unknown email skips
bcrypt, so it answers faster. Counting made-up emails would let them fill Redis. Hiding
which emails exist would need `signup` and the bcrypt timing changed too: a separate issue.
