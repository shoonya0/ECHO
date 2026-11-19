# Server changes: auth, connection and errors

Part of [server changes the app must handle](./README.md).

## Presence: offline arrives, once per connection (#31)

- `presence` events reach every connection that shares a chat with the user, on any
  server, **once**, however many chats they share. Before, there was one per shared chat,
  and `offline` never arrived.
- `offline` comes when the user's last connection on any server closes. While they are
  still connected to another server, nothing is sent.
- `online` can come twice for one user (connected to two servers): treat a repeat as no
  change. `data.instanceID` is gone.
- A user who chose `invisible` gets no `online` event. `away` and `dnd` users do.
- REST `presence` (contacts, your profile, `login`) is live now (#31, part 2):
  - `isOnline` is whether they are connected. For others, `status` is `offline` while they
    aren't connected or are invisible; otherwise their choice (`online`, `away`, `dnd`).
  - `lastSeen` is when their last connection closed (empty for invisible users).
  - Your own `status` is what you set. `PUT profile/` ignores `isOnline` and `lastSeen`:
    stop sending them.

Details: [websocket-events.md](../../../_docs/websocket-events.md#presence-someones-online-status-changed),
[users.md](../../../_docs/rest-api/users.md#profile).

## Deleting an account (#35)

- After `DELETE profile/delete`, the deleted person's messages and direct-chat entry show
  `displayName` "Deleted user" with an empty `username` and `avatar`: show that name, and
  don't open a profile for them (`GET users/:id` answers `404`).
- They are gone from your contacts and from groups. A group they owned gets a new owner.

Details: [users.md](../../../_docs/rest-api/users.md#profile).

## Emails ignore case; profile can't change the email (#36)

- `signup` and `login` lowercase the email, so `Alice@x.com` and `alice@x.com` are one
  account. A signup with other capitals of a taken email gets `409`.
- `PUT profile/` ignores `email` (it still answers `200` for the other fields). Hide or
  disable email editing in the profile screen until a change-email flow exists.

Details: [users.md](../../../_docs/rest-api/users.md#accounts).

## Unknown chats look like chats you aren't in (#59)

- `send_message`, `invite_user` and `remove_user` now answer `PERMISSION_DENIED` for a chat
  you aren't in, whether or not it exists. Before, an unknown chat got `INVALID_CHAT_ID` or
  `PERMISSION_CHECK_FAILED`, and `invite_user` to a direct chat got `INVALID_REQUEST`.
- `INVALID_CHAT_ID` now only means a badly formed chat ID. `remove_user` on a direct chat
  gets `INVALID_REQUEST`, as `invite_user` does.
- Don't drop a chat from the UI on `PERMISSION_DENIED`: members get it too, for a missing
  role, muting or a block. Refresh the chat list instead; a chat you left won't be in it.
- REST `add-members` returns `404` (was `403`) if you aren't in the group.

Details: [WEBSOCKET_PROTOCOL.md](../../../_docs/WEBSOCKET_PROTOCOL.md#error-codes).

## `join_chat` follows any chat you are in (#48)

- `join_chat` with `chatId` now follows that chat on this connection, groups included; a
  chat you aren't in gets `PERMISSION_DENIED`. Before, `chatId` was ignored and the request
  failed with `INVALID_USER_ID`. `senderId` still opens a direct chat.
- Both now answer with a `response` whose `data.chatId` names the chat: wait for it like
  `send_message`, instead of waiting for the `join` event. A chat the socket already
  follows gets no second `join` event, so joining again on reconnect is safe.

Details: [websocket-requests.md](../../../_docs/websocket-requests.md#join_chat).

## Login and signup limits (#40, PR #139, merged 2026-11-18)

- `login` and `signup` can answer `429` with `error` `RATE_LIMITED` and a `Retry-After`
  header (seconds). After 10 attempts on one account from the same network in 15
  minutes, even the right password gets `429` until the wait ends.
- Show "too many attempts, try again in N minutes" using `Retry-After`. Don't retry
  automatically, and don't treat it as a wrong password.

Details: [users.md](../../../_docs/rest-api/users.md#accounts).

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

Details: [rest-api/](../../../_docs/rest-api/README.md).

## WebSocket close codes (#76)

The server now closes an open WebSocket when its session ends, with a close frame that
says why. The reasons are the REST codes above.

| Close code | Reason | What the app should do |
|---|---|---|
| `4401` | `TOKEN_REVOKED` | Logged out (this token only; other devices stay connected). Don't reconnect. |
| `4401` | `TOKEN_EXPIRED` | The 7-day token ran out: log in again. |
| `4401` | `ACCOUNT_NOT_FOUND` | Log out and clear local data. |
| `4403` | `ACCOUNT_BANNED` / `ACCOUNT_INACTIVE` | Log out and show why. |
| `1011` | `try again` | The server couldn't set up the connection (#22). Reconnect with backoff. |
| anything else | | Reconnect with backoff, as before. |

- The reconnect logic must check the close code first: reconnecting after a `4401` or
  `4403` only gets a `401` or `403` at the upgrade.
- A ban made in the database closes the socket within about a minute.
- When a user comes online (their first connection on a server), the app now also gets
  its own `presence` event (`online`), once (#31); before #22 it was usually lost.
  Further connections of an online user don't announce it again. Ignore presence for the
  user's own ID if the app doesn't use it.

Details: [WEBSOCKET_PROTOCOL.md](../../../_docs/WEBSOCKET_PROTOCOL.md#stay-connected).

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

Details: [rest-api/](../../../_docs/rest-api/basics.md#the-basics),
[WEBSOCKET_PROTOCOL.md](../../../_docs/WEBSOCKET_PROTOCOL.md#error-codes).

## WebSocket limits (#40)

| Server behaviour | What the app should do |
|---|---|
| A frame over 64 KiB closes the socket with `1009` | Keep requests small (message content is at most 4000 bytes anyway). Treat `1009` as a bug, not a reason to retry the same frame. |
| Over 20 requests a second (bursts of 40, pings included), a request isn't carried out. It gets an `error` with code `RATE_LIMITED` and its `requestId`, or no reply at all while the server's queue for this socket is busy | Debounce `set_typing` and batch `mark_read`. On `RATE_LIMITED`, or a request timeout, retry after a short backoff. The socket stays open. Don't send WebSocket pings more often than every few seconds. |
| `set_typing` only sends the `typing` event; a chat's `typingUsers` in REST replies is no longer updated and can hold old entries | Show typing only from `typing` events, and clear it after a few seconds without a new one. Ignore `typingUsers`. |

Details: [WEBSOCKET_PROTOCOL.md](../../../_docs/WEBSOCKET_PROTOCOL.md#stay-connected).
