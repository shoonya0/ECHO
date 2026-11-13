# ECHO Android: server changes the app must handle

The server changed after the plans in this folder were written. Read this page before
working on the app's networking or login code. Each entry says what the server does now,
what the app should do, and where the details are.

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

- REST: the `error` field of a `400` is now `"field: rule; field: rule"` (JSON field
  names) or `"request body is not valid JSON for this endpoint"`, not Gin's raw text. Show
  it in debug builds only; for users, map the endpoint and status to your own text.
- WebSocket: messages are fixed texts. `invite_user` failures now come with
  `INVALID_CHAT_ID`, `PERMISSION_DENIED`, `INVALID_USER_ID` or `INVALID_REQUEST` (already in
  the chat) instead of always `INVITE_FAILED`. Branch on `code`.

Details: [rest-api.md](../../_docs/rest-api.md#the-basics),
[WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md#error-codes).

## WebSocket limits (#40)

| Server behaviour | What the app should do |
|---|---|
| A frame over 64 KiB closes the socket with `1009` | Keep requests small (message content is at most 4000 bytes anyway). Treat `1009` as a bug, not a reason to retry the same frame. |
| Over 20 requests a second (bursts of 40, pings included), a request isn't carried out. It gets an `error` with code `RATE_LIMITED` and its `requestId`, or no reply at all while the server's queue for this socket is busy | Debounce `set_typing` and batch `mark_read`. On `RATE_LIMITED`, or a request timeout, retry after a short backoff. The socket stays open. Don't send WebSocket pings more often than every few seconds. |

Details: [WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md#stay-connected).
