# ECHO decisions: accounts, contacts and privacy

Up: [decisions](./README.md)

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

## How blocks are stored

Decided on 2026-11-08 (#26). A block is stored on both people, with direction:
`contactInfo.blocked` on the blocker and `contactInfo.blockedBy` on the blocked person.
`blockedBy` is never sent to clients. Old `blockedChats` data has no direction, so the
startup migration (`MigrateLegacyBlocks`) turns each pair into a mutual block and removes
the old field. Nobody is unblocked without choosing to be.

## Changing the email (#36)

Decided on 2026-11-18. `PUT profile/` no longer changes `email`. It took any string, with
no password or format check, so a stolen token could move the account to another
address, and a taken email answered `500`. Emails are stored trimmed and in lower case.
Signup and login match them ignoring case (a collation index, `email_ci`), so emails
stored with capitals before #36 work without waiting for the startup migration that lowers
them. Two old accounts whose emails differ only in case are both kept: login picks the one
stored exactly as typed, and the migration logs them for a manual merge. A change-email
flow, with the current password and a check of the new address, is a separate issue.

## Unique usernames (#36)

Decided on 2026-11-19. A username is unique ignoring case (`Alice` and `alice` are one
name) and keeps the capitals it was typed with. A new or changed name is 3 to 30 letters,
digits, `_` or `.`, so it works as an @handle; names from before #36 stay until their owner
changes them. A unique collation index, `username_ci`, enforces it. Accounts that shared a
name before #36 are renamed at startup (`RenameDuplicateUsernames`, before the indexes): the
oldest account keeps the name, each later one gets the first free `<name>_2`, `<name>_3`,
and every rename is logged. The renamed person isn't told; copies of the old name in chats
and messages stay stale like any profile change, which is the rest of #36.

## Where presence comes from (#31)

Decided on 2026-11-19. Whether someone is connected comes from Redis `online:<userID>`
(a mark per server, renewed every 30 s, expiring after 2 minutes), read when a reply is
built; the stored `presence.isOnline` is never read. `presence.status` is the status the
user chose; the hub reads it from MongoDB at connect, not from the token, which can be 7
days old, and treats a failed read as `invisible`. `away` and `dnd` users are announced with
that status; `invisible` (or `offline` by choice) users get neither online nor offline
events, and others see them as `offline` with no `lastSeen`. Other statuses answer `400`. The server writes
`presence.lastSeen` when it publishes the offline, with `$max` so writes can't go back;
`PUT profile/` can no longer set `isOnline` or `lastSeen`. When Redis can't answer, replies
show everyone offline rather than failing. A status change while connected isn't announced
live yet. Privacy settings narrow this further: see below.

## How privacy settings apply (#32)

Decided on 2026-11-19. `settings.privacy` takes effect everywhere presence is shown, REST
and events alike. `showOnlineStatus` `contacts` shows the user only to accepted contacts
(contacts are mutual, so the viewer's own list decides); its events carry the contact IDs
and each server delivers them to those users only. `none`, or `contacts` with no contacts,
announces the user to nobody, as `invisible` does. A user hidden from a viewer shows no
`lastSeen` to them either. `showLastSeen` `false` hides `lastSeen` and `lastActivity`, and
leaves `lastSeen` out of events. It was stored `false` for every account while nothing read
it, so new accounts get `true` and a startup migration (marker
`show-last-seen-default-32`, claimed before the update) turned it on once for old ones;
later choices stay. The online uses the settings read at connect; the offline reaches the
same people but reads them again and leaves `lastSeen` out unless they still allow it.
`allowContactBy` `none` refuses every contact request (`403`); `contacts` lets in people who
share a group with you as accepted members. Pending invitees don't count, and neither do
direct chats, since anyone can open one.

Also decided on 2026-11-19, to keep the work small: `allowContactBy` doesn't gate opening a
direct chat (`CreateDirectChat`), and a status or privacy change made while connected
isn't sent live (no `set_presence` handler); it reaches events at the next connect. REST
replies already show the change at once. Revisit if users ask for either. A value the server doesn't know means nobody, so `PUT
profile/` answers `400` for anything but `everyone`, `contacts` or `none`.

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
