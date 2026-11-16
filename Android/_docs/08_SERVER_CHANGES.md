# ECHO Android: server changes the app must handle

The server changed after the plans in this folder were written. Read this page before
working on the app's networking or login code. Each entry says what the server does now,
what the app should do, and where the details are.

## Groups: size limit, count, accepting (#68, #16, #103)

- A group holds at most 100 people. A create, add or join by code past that adds nobody:
  REST `409` `Group is full`, WebSocket `GROUP_FULL`. Show "group is full"; don't retry.
- `stats.participantCount` is now correct, but counts pending members (#5) and deleted accounts (#35).
- A `pending` member accepts by joining with a code (`200`; the invite is cleared). No decline yet.

Details: [rest-api.md](../../_docs/rest-api.md#groups).

## Blocked people aren't in the contacts list (#62)

- `GET users/contacts` no longer lists people you blocked: show them from
  `GET users/contacts/blocked`, and drop them from cached contacts after a block
  ([details](../../_docs/rest-api.md#contacts)).

## Creating an invite code returns it (#104)

- `POST groups/:groupID/invites` now returns the new code in `data`, in the same shape as
  an entry of `GET groups/:groupID/invites` (`inviteCode`, `expiredDate`, `deleted`, ...).
  Share `data.inviteCode` directly; don't take the first entry of the list, which is the
  code made when the group was created.

Details: [rest-api.md](../../_docs/rest-api.md#groups).

## Unknown chats look like chats you aren't in (#59)

- `send_message`, `invite_user` and `remove_user` now answer `PERMISSION_DENIED` for a chat
  you aren't in, whether or not it exists. Before, an unknown chat got `INVALID_CHAT_ID` or
  `PERMISSION_CHECK_FAILED`, and `invite_user` to a direct chat got `INVALID_REQUEST`.
- `INVALID_CHAT_ID` now only means a badly formed chat ID. `remove_user` on a direct chat
  gets `INVALID_REQUEST`, as `invite_user` does.
- Don't drop a chat from the UI on `PERMISSION_DENIED`: members get it too, for a missing
  role, muting or a block. Refresh the chat list instead; a chat you left won't be in it.
- REST `add-members` returns `404` (was `403`) if you aren't in the group.

Details: [WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md#error-codes).

## Auth replies (#33, PR #77, live on Render since 2026-11-11)

Every request now checks the account itself, not only the token. A token stops working
when its account is banned, deactivated or deleted after login.

| Reply | When | What the app should do |
|---|---|---|
| `403` `ACCOUNT_BANNED` | The account was banned | Log out and show "account banned". Logging in again also returns `403`. |
| `403` `ACCOUNT_INACTIVE` | The account was deactivated | Log out and show "account not active". |
| `401` `ACCOUNT_NOT_FOUND` | The profile was deleted | Log out and clear local data for that account. |
| `401` `TOKEN_REVOKED`, `TOKEN_VERIFICATION_FAILED`, … | Logged out, expired or bad token | Log out, as before. |
| `503` `AUTH_UNAVAILABLE` | The server can't check the token right now (Redis or MongoDB down) | **Don't log out.** Keep the session and retry later with backoff. |

- These replies have the token-error shape `{ "error": "...", "code": "..." }`, not the
  usual `{ "success": false, ... }`. Branch on `code`, not on the message text.
- The app's networking code must not treat every non-`200` reply as "session expired".
  Only `401` and the two `403` codes above mean the session is over.
- A ban made in the database takes up to 30 seconds to apply. Deleting the profile
  applies at once.
- The WebSocket closes when the session ends; see below.

Details: [rest-api.md](../../_docs/rest-api.md).

## WebSocket close codes (#76)

The server now closes an open WebSocket when its session ends, with a close frame that
says why. The reasons are the REST codes above.

| Close code | Reason | What the app should do |
|---|---|---|
| `4401` | `TOKEN_REVOKED` | Logged out (this token only; other devices stay connected). Don't reconnect. |
| `4401` | `TOKEN_EXPIRED` | The 7-day token ran out: log in again. |
| `4401` | `ACCOUNT_NOT_FOUND` | Log out and clear local data. |
| `4403` | `ACCOUNT_BANNED` / `ACCOUNT_INACTIVE` | Log out and show why. |
| anything else | | Reconnect with backoff, as before. |

- The reconnect logic must check the close code first: reconnecting after a `4401` or
  `4403` only gets a `401` or `403` at the upgrade.
- A ban made in the database closes the socket within about a minute.

Details: [WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md#stay-connected).

## Error details (#41)

- REST: the `error` field of a `400` is now `"field: rule; field: rule"` (JSON paths, such
  as `password: min=8`), `"age: wrong type"`, or `"request body is not valid JSON for this
  endpoint"`, not Gin's raw text. Show it in debug builds only; for users, map the endpoint
  and status to your own text.
- WebSocket: messages are fixed texts. `invite_user` failures now come with
  `INVALID_CHAT_ID`, `PERMISSION_DENIED` (not a member), `INVALID_USER_ID` or
  `INVALID_REQUEST` (already in the chat). `INVITE_FAILED` is left for the rest, including
  a conflict with another change to the chat: retry it. `INVALID_CONTENT` for a bad
  `messageType` no longer repeats the value. Branch on `code`.

Details: [rest-api.md](../../_docs/rest-api.md#the-basics),
[WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md#error-codes).

## Invite codes (#15)

- New invite codes are 32 random hex characters. Before, they were much longer and held the
  group ID and the creator's ID and name, encrypted with a key anyone could read. Store and
  show a code as an opaque string; don't parse it or assume its length.
- Codes made before the change keep working until they expire (24 hours).
- A code that isn't stored on any group now gets `400 Invalid invite` on every invite
  route, not `200` or `500`.
- Sending a deleted code to someone (`POST /groups/invites/:code/:userID/send`) now gets
  `400 Invalid invite` too, as an expired one already did (#91).

Details: [rest-api.md](../../_docs/rest-api.md#groups).

## Only owners and admins manage invite codes (#30)

- Creating, listing, deleting and sending invite codes, and "who joined", now return `403`
  for members who aren't the owner or an admin, and `404` for anyone not in the group or a
  group that doesn't exist. Show
  those actions only to owners and admins. Joining with a code is unchanged.
- The list includes deleted and expired codes; check `deleted` and `expiredDate`.
- Sending a code returns `404` for an unknown user and `409` for someone already in the
  group (both were `500`). Sending it again doesn't add a second invitation, and joining
  removes the invitation from `GET groups/invites`. People added when the group was created
  are pending members: they can't join by code, so their invitation stays listed.

Details: [rest-api.md](../../_docs/rest-api.md#groups).

## Removing group members (#28)

- `remove_user` refuses to remove the group's owner unless the owner sends it: the reply is
  `PERMISSION_DENIED` and nobody in the request is removed. Hide "Remove" on the owner for
  admins.
- If your role or the group changed while the request ran, the reply is also
  `PERMISSION_DENIED`. Reload the group before retrying.
- Listing yourself among the users no longer takes the group off your own chat list.
- The reply's `removedUsers` now lists only the people removed. Before, it echoed every ID
  sent, including you and people not in the group; update the member list from it.

Details: [websocket-requests.md](../../_docs/websocket-requests.md#remove_user).

## Yourself as a target (#38)

- A contact request to yourself, or `POST chats/direct/<your ID>`, now gets `400`. Before,
  it went through and stored broken data. `join_chat` with your own `senderId` gets
  `INVALID_USER_ID`. Don't offer "Add contact" or "Message" on your own profile.

Details: [rest-api.md](../../_docs/rest-api.md#contacts).

## Blocks apply in direct chats (#27)

- If either person blocked the other, `POST chats/direct/<ID>` gets `403` `User is
  blocked`, and `join_chat` gets `PERMISSION_DENIED` with the same message. This holds
  for a chat that already exists.
- `send_message`, `set_typing`, `mark_read`, `add_reaction` and `remove_reaction` in
  that direct chat get `PERMISSION_DENIED` `User is blocked`, for both the blocker and
  the blocked person. Nothing is stored or delivered. Don't send typing or read events
  for a blocked chat.
- The chat and its history stay in both chat lists. Show it as read-only with a "blocked"
  note instead of an error, and hide the message box. Unblocking makes it work again.
- Group chats aren't affected.

Details: [websocket-requests.md](../../_docs/websocket-requests.md).

## Reactions count each user once (#25)

- `add_reaction` with an emoji you already reacted with, or `remove_reaction` with one you
  didn't, still gets its `response`, but changes nothing and sends no `reaction` event.
  Before, it changed the count and sent an event anyway. The `response` now has
  `changed`: `true` or `false`. Only update the reaction locally when it is `true`.
- So each `reaction` event is a real change: add or remove `userId` for that emoji. A
  remove by someone else can no longer delete other people's reactions.
- In history, `reactions.<emoji>.count` equals the length of `users`. Old messages whose
  count drifted are corrected the next time someone reacts with that emoji. Prefer
  `users` over `count` when they disagree.

- `reactions.<emoji>.details` maps each user ID in `users` to that user's username
  (#75), so history can show who reacted. Reactions stored before then have `""`; look
  those users up by ID. The name is a copy taken when they reacted, so it can be out of
  date after a username change.

Details: [websocket-requests.md](../../_docs/websocket-requests.md#add_reaction-and-remove_reaction).

## `send_message` replies have a top-level `requestId` (#47)

- The `response` to `send_message` now has `requestId` at the top level, like every other
  `response` and `error`. Before, it was only in `data.requestId`, so the plan's
  `frame.requestId ?: return` dropped every `send_message` reply and the send timed out.
- Match replies on the top-level `requestId`. `data.requestId` is still there.

Details: [websocket-events.md](../../_docs/websocket-events.md).

## WebSocket limits (#40)

| Server behaviour | What the app should do |
|---|---|
| A frame over 64 KiB closes the socket with `1009` | Keep requests small (message content is at most 4000 bytes anyway). Treat `1009` as a bug, not a reason to retry the same frame. |
| Over 20 requests a second (bursts of 40, pings included), a request isn't carried out. It gets an `error` with code `RATE_LIMITED` and its `requestId`, or no reply at all while the server's queue for this socket is busy | Debounce `set_typing` and batch `mark_read`. On `RATE_LIMITED`, or a request timeout, retry after a short backoff. The socket stays open. Don't send WebSocket pings more often than every few seconds. |

Details: [WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md#stay-connected).
