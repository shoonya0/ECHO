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

## Where chat membership is stored

Decided on 2026-11-15. A chat's `participants` is the only record of who is in it.
`users.chats` isn't trusted for membership; it is dropped, or rebuilt from `participants`.
`GetUserChats` already checks the list against `participants`. Unblocks #14 and #17.

## What deleting an account does

Decided on 2026-11-15. Soft delete and anonymize: the account is marked deleted, its email,
username and profile are cleared (so the username is free again), and its messages show
as "Deleted user". Other people's chat history stays whole. The auth check already refuses
a deleted account (`ACCOUNT_NOT_FOUND`). Unblocks #35.

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

## How login and signup limits find the client (#40)

Decided on 2026-11-17. Failed logins are counted per email, from any IP: that count can't
be dodged and protects the account. Logins and signups are also counted per client IP.
The IP comes from the header named in `CLIENT_IP_HEADER` (`True-Client-IP` on Render, set
by Cloudflare in front of it), or else the socket address. `X-Forwarded-For` is never
trusted, because Render's proxy appends to what the client sent. Per-IP limits are
generous because mobile carriers put many users behind one address. Counts live in Redis.
When Redis is down, requests go through: an outage shouldn't block every login.
