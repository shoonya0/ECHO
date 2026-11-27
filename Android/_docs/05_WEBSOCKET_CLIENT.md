# ECHO Android — WebSocket Client

How the shipped app ([ECHO-app](https://github.com/shoonya0/ECHO-app)) uses the server's
WebSocket. The protocol is owned by the server docs: start at
[WEBSOCKET_PROTOCOL.md](../../_docs/WEBSOCKET_PROTOCOL.md), then
[websocket-requests.md](../../_docs/websocket-requests.md) and
[websocket-events.md](../../_docs/websocket-events.md) for exact frames. This page covers
what the app does with them; it doesn't repeat their frame formats.

## One socket per app run

`EchoWebSocketClient` is a `@Singleton` and owns **one** socket. `connect()` does nothing
while a socket is open or connecting, and callbacks from an older socket are ignored, so
any number of screens can call it safely.

| Moment | What the app does |
|---|---|
| Home (chat list) appears after login, or Splash finds a valid token | `ChatListViewModel.init` → `connect()` — **this is where the socket opens** |
| A chat detail screen opens | `connect()` again (no-op), then `join_chat` for that chat |
| Back from a chat | `leave_chat` only — **the socket stays open** |
| Another chat opens | `join_chat` on the same socket |
| Going to Home, contacts or settings | nothing |
| App goes to background | nothing — there is no disconnect-on-stop hook |
| Logout, token expiry, ban or account deletion | the server closes the socket; the app does not reconnect |
| Network drop or process death | the socket is gone; the app reconnects with backoff (missed messages are not replayed) |

`ChatDetailViewModel` also calls `connect()` before joining, so opening a chat works even
if Home has not composed.

The server follows every chat in the user's chat list when a socket connects, so
`join_chat` matters for chats gained after connecting (e.g. a group joined with an invite
code, #49) and for direct chats opened by `senderId`. One account may hold 5 connections
per server (server #40), so one socket per device is the design.

```
Splash / Login ──valid token──▶ Home composed
                                  └─ ChatListViewModel.init ─ connect() ─▶ socket OPEN
Chat detail opens ─ connect() (no-op) + join_chat
Back             ─ leave_chat        (socket stays OPEN)
Session ends     ─ disconnect()      ─▶ socket CLOSED, no reconnect
```

## Endpoint and authentication

```
ws://{host}/echo/v1/ws/chat      # Debug (Android emulator: ws://10.0.2.2:8080)
wss://{host}/echo/v1/ws/chat     # Release
```

The login token goes in the `Authorization: Bearer <token>` header during the upgrade.
The server accepts it nowhere else (not in the URL, not in a cookie).

```kotlin
val request = Request.Builder()
    .url("${BuildConfig.WS_BASE_URL}/echo/v1/ws/chat")
    .header("Authorization", "Bearer $token")
    .build()
```

The first frame on every socket is the server's welcome: a `response` with
`data.status == "connected"`, `clientId` and `instanceId`, and no `success` field. It shares
the `response` type with request replies, so the app recognizes it before decoding
(`isWelcomeFrame()`) and ignores it (#30).

## Requests the app sends

| Request | Sent when | Reply |
|---|---|---|
| `send_message` | the user sends a message | correlated `response` (10 s timeout); the `message` event also arrives |
| `join_chat` | a chat opens, or a new direct chat is created | fire-and-forget (`sendRaw`); the server's `response` and `join` arrive as events |
| `leave_chat` | the chat screen is destroyed (back) | none; it only stops live events on this connection — the user stays a member |
| `set_typing` | typing starts and periodically while it continues | none; throttled so the socket's rate limit (#40) isn't hit |
| `mark_read` | while the chat is visible, batched, one in flight | none; a lost request goes out again after a reconnect |
| `add_reaction` / `remove_reaction` | the user taps a reaction | fire-and-forget (`sendRaw`); the server's `response` arrives as an event |

`join_chat` normally carries `chatId`. The app also sends `senderId` (the current user),
but the server ignores it when `chatId` is present. A brand-new direct chat is created over
REST (`POST chats/direct`) first, then joined by `chatId`.

Blocked direct chats: the server refuses sends, typing, reads and reactions with
`PERMISSION_DENIED` (`User is blocked`), for both people; see
[websocket-requests.md](../../_docs/websocket-requests.md).

## Events the app handles

| Event | App behavior |
|---|---|
| `message` | `DomainEvent.NewMessage`; reconciles the optimistic bubble for our own echoed message, updates the chat list/detail |
| `typing` | `DomainEvent.TypingUpdate` |
| `presence` | `DomainEvent.PresenceUpdate`; drives the chat list and contact dots |
| `reaction` | `DomainEvent.ReactionUpdate` |
| `delivery` | `DomainEvent.ReadReceipt(chatId, userId, readUpToSeq)`; `readUpToSeq` never goes down and marks every message up to it as read (#42) |
| `join` / `leave` | `DomainEvent.UserJoined` / `UserLeft` |
| `response` | resolves the pending request with the same `requestId` |
| `error` | `DomainEvent.BackendError(code, message, requestId)`; screens show mapped text via `ErrorMapper` |

## Implementation

### `EchoWebSocketClient` (`features/chat/data/remote`)

- Holds `webSocket` and `isOpen` under a lock; OkHttp callbacks take the same lock.
- No token → emits `ReconnectFailed` and stops, instead of an error that would start an
  endless reconnect loop.
- A refused upgrade with HTTP `401`/`403` is reported as a `4401`/`4403` disconnect,
  reading the server's `code` from the response body; the session then ends.
- `onClosing` acknowledges the close with `1000`, then forwards the server's code and reason.
- `disconnect()` closes with `1000`, clears the socket, and ignores its later callbacks,
  so no `Disconnected` event and no reconnect follow.

### `ReconnectManager` (`features/chat/data/remote`)

- Up to 5 attempts, delays `1 s → 2 s → 4 s → 8 s → 16 s` plus `0–1000 ms` jitter, one
  pending job at a time.
- Any successful `Connected` resets the counter; exhausting the retries emits
  `ReconnectFailed`, which the repository reports as `WsConnectionStatus.FAILED` (the
  detail screen shows "Connection lost — check your network").
- `stop()` cancels a scheduled retry and resets the counter on `disconnect()` and on a
  session-ending close.
- Close codes `4401` and `4403` (`SESSION_ENDED_CODES`) never reconnect: reconnecting
  would only get a `401`/`403` at the upgrade. The session is ended instead.

### `WebSocketRepositoryImpl` (`features/chat/data/repository`)

- `send(command)` registers a `CompletableDeferred` by `requestId` and waits up to 10 s for
  a `response` or `error`. Only those two frame types resolve a pending request; broadcast
  frames carrying the same `requestId` (e.g. our own echoed `message`) are still emitted as
  domain events, so the sent bubble appears.
- `sendRaw(command)` is fire-and-forget and returns false when the socket isn't open.
- Publishes `WsConnectionStatus`: `CONNECTING` on `connect()`, `CONNECTED` on open,
  `DISCONNECTED` on a non-session close, `FAILED` after the retries run out.
- A session end from either side — a `4401`/`4403` close, or a REST `401`/`403` on
  `SessionManager.sessionExpired` — disconnects without reconnecting and fails pending
  requests.
- The welcome frame is consumed before `ServerFrame` decoding (#30). Unparseable frames are
  logged and dropped, never crash the socket.

### Models (`features/chat/domain/model`)

- `WebSocketCommand`: `send_message`, `join_chat`, `leave_chat`, `set_typing`, `mark_read`,
  `add_reaction`, `remove_reaction`; each takes a random 8-character `requestId` by default.
- `ServerFrame`: `message`, `typing`, `presence`, `reaction`, `delivery`, `join`, `leave`,
  `response`, `error`. `MessageData` carries `seq`; `DeliveryData` requires `readUpToSeq`
  and `ResponseData` can carry the new message's `seq` in `data`.
- `DomainEvent`: the app-facing types listed in the events table above.

## Session end and close codes

| Close / upgrade | Meaning | App response |
|---|---|---|
| `4401` `TOKEN_REVOKED` / `TOKEN_EXPIRED` / `ACCOUNT_NOT_FOUND` | this session is over | stop reconnecting, log out (reason kept) |
| `4403` `ACCOUNT_BANNED` / `ACCOUNT_INACTIVE` | this session is over | stop reconnecting, log out with the reason |
| HTTP `401` / `403` at the upgrade | the same as `4401` / `4403` | the same |
| `1000` | the app asked to close | no reconnect |
| `1008` / HTTP `429` once 5 connections exist | connection limit | retry only through the normal backoff |
| `1009` | a frame was too big | reconnects; treat it as a bug and don't resend that frame |

Server details: [auth-and-connection.md](./08_SERVER_CHANGES/auth-and-connection.md#websocket-close-codes-76).

## Keepalive and limits

- The client sets `pingInterval(54 s)`, the same cadence as the server's pings; OkHttp
  answers the server's pings automatically. The server closes a socket after 60 s without a
  pong, and a cleanup job drops sockets idle for 5 minutes.
- Frames over 64 KiB close with `1009`; more than 20 requests a second (bursts of 40) get
  `RATE_LIMITED` or no reply for a moment. The app debounces `set_typing` and batches
  `mark_read` because of this.
- After a reconnect, load recent messages over REST: the server doesn't replay what was
  missed (#43).

## Configuration

```kotlin
// In NetworkModule — a separate client from REST
@Provides @Singleton @WsClient
fun provideWsOkHttpClient(): OkHttpClient = OkHttpClient.Builder()
    .pingInterval(54, TimeUnit.SECONDS)
    .readTimeout(0, TimeUnit.MILLISECONDS)   // no read timeout for the WebSocket
    .connectTimeout(10, TimeUnit.SECONDS)
    .build()
```
