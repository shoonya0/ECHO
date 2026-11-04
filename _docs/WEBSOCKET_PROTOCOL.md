# WebSocket protocol

The WebSocket is how live chat works in ECHO. An app opens one connection and keeps it
open. The app sends **requests**, and the server pushes **events** back.

This page is the overview. For exact formats, see:

- [websocket-requests.md](./websocket-requests.md): everything an app can send
- [websocket-events.md](./websocket-events.md): everything the server sends
- [websocket-client-example.md](./websocket-client-example.md): working client code

## Connect

```
GET /echo/v1/ws/chat
```

Locally, the address is `ws://localhost:8080/echo/v1/ws/chat`.

Send your login token in the `Authorization: Bearer <token>` header. The server accepts it
nowhere else: not in the URL, not in a cookie. A plain browser page can't set this header,
so it can't connect. The Android app (OkHttp), Node (`ws`) and `wscat -H` all can.

When you connect, the server:

1. Checks your token.
2. Starts following every chat in your user's chat list.
3. Sends you a welcome `response` with `"status": "connected"`.

## Stay connected

- The server sends a ping every 54 seconds. Your client must answer with a pong. Most
  WebSocket libraries do this for you.
- With no pong for 60 seconds, the server closes the connection.
- A cleanup job also removes connections with no activity for 5 minutes.
- The server queues up to 10 outgoing messages per connection. If your app reads too
  slowly and the queue fills up, the server disconnects it.

After a reconnect, load recent messages over REST. The server doesn't replay what you
missed (#43).

## Which chats you get live events for

**Being in a chat and following it live are two different things.** Sending and reading
history depend on the chat's `participants`. Live events depend on what your connection
follows.

A connection follows:

- The chats in your user's `chats` list at the moment you connect.
- Direct chats you open with `join_chat` on that connection.

So these cases get no live events until you reconnect:

- Someone opens a new direct chat with you while you are online.
- You create a group, or join one with an invite code.

Two more cases to know about:

- Members added to a group get no live events at all, even after reconnecting (#17).
- Members removed from a group keep getting live events until they reconnect (#14).

## Message format

Requests are JSON text frames. The `type` says what you want. `requestId` is optional; it
is your own label, so you can match a reply to its request.

```json
{ "type": "send_message", "chatId": "<chatId>", "content": "Hi", "requestId": "r1" }
```

Events have this shape. Fields with no value are left out.

```json
{
  "type": "message",
  "chatId": "<chatId>",
  "userId": "<userId>",
  "data": {},
  "requestId": "r1",
  "timestamp": "2026-11-04T12:00:00Z"
}
```

## Requests at a glance

**Not every request gets a `response`.** If you wait for one where there is none, you will
wait forever. A request that is rejected gets an `error` instead. A problem after that
point, such as a failed live broadcast, may not be reported at all.

| Request | What it does | On success you get |
|---|---|---|
| `send_message` | Sends a message | `response`, and a `message` event |
| `join_chat` | Opens a direct chat with someone | A `join` event, no `response` |
| `leave_chat` | Stops live events for a chat on this connection | A `leave` event, no `response` |
| `set_typing` | Says you are typing or stopped | A `typing` event, no `response` |
| `mark_read` | Marks messages as read | A `delivery` event, no `response` |
| `add_reaction` | Adds an emoji reaction | `response`, and a `reaction` event |
| `remove_reaction` | Removes an emoji reaction | `response`, and a `reaction` event |
| `invite_user` | Adds people to a group (owner or admin) | `response`, and a `chat_updated` event |
| `remove_user` | Removes people from a group (owner or admin) | `response`, and a `chat_updated` event |
| `edit_message` | Not built | `NOT_IMPLEMENTED` error |
| `delete_message` | Not built | `NOT_IMPLEMENTED` error |
| `update_chat` | Not built | `NOT_IMPLEMENTED` error |

`set_presence` exists in the models but has no handler, so it returns `UNKNOWN_REQUEST`.
Old messages are loaded [over REST](./rest-api.md#messages), not over the WebSocket.

## Events at a glance

| Event | When it is sent |
|---|---|
| `message` | Someone sent a message |
| `typing` | Someone started or stopped typing |
| `delivery` | Someone read messages |
| `reaction` | Someone added or removed a reaction |
| `join`, `leave` | Someone started or stopped following a chat |
| `presence` | Someone came online or went offline. Often missing (#31). |
| `chat_updated` | Group members changed |
| `response` | Your request worked, or the welcome message |
| `error` | Your request failed |

## Chat types and who can send

| Type | Who is in it | Who can send |
|---|---|---|
| `direct` | Two people | Both |
| `group` | The creator plus at least one other person. A limit of 100 is stored but not checked (#42). | Everyone in `participants`, including pending members (#5). If a member has a `permissions` list, it must include `write`, `send_message`, `admin` or `owner`. |
| `channel` | Planned. Nothing can create one yet. | The code would allow only `admin` and `owner`. |

- Groups get a name and description when created. They can't be changed afterwards,
  because `update_chat` isn't built.
- The code refuses participants marked `isBlocked` or `isMuted`, but nothing sets these
  flags yet. Blocking someone in your contacts has no effect in chats (#27).

## Error Codes

An `error` event has a `code` and a readable `message`.

| Code | Meaning |
|---|---|
| `INVALID_REQUEST` | A required field is missing or wrong |
| `INVALID_CHAT_ID` | The chat ID has the wrong format, or the chat wasn't found |
| `INVALID_MESSAGE_ID` | The message ID has the wrong format |
| `INVALID_USER_ID` | The user ID has the wrong format |
| `MESSAGE_NOT_FOUND` | The message doesn't exist |
| `PERMISSION_DENIED` | You aren't allowed to do this |
| `PERMISSION_CHECK_FAILED` | The server couldn't check your permission, for example because the chat doesn't exist |
| `INVALID_CONTENT` | The message broke a content rule, such as being too long |
| `MESSAGE_FAILED` | The message couldn't be sent |
| `REACTION_FAILED` | The reaction couldn't be added or removed |
| `READ_FAILED` | Messages couldn't be marked as read |
| `TYPING_FAILED` | Typing status couldn't be updated |
| `CHAT_CREATION_FAILED` | The chat couldn't be created |
| `INVITE_FAILED` | People couldn't be added to the group |
| `REMOVE_FAILED` | People couldn't be removed from the group |
| `NOT_IMPLEMENTED` | This feature isn't built yet |
| `UNKNOWN_REQUEST` | The server doesn't know this request type |
| `PARSE_ERROR` | Your message wasn't valid JSON |
| `CHANNEL_FULL` | Your outgoing queue is full, so a reply was dropped |
| `USER_NOT_AUTHENTICATED` | The server couldn't find your user |

Some error messages still contain internal text (#41). Build your app on `code`, not on
`message`.
