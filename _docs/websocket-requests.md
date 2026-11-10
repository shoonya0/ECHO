# WebSocket requests

Everything an app can send over the WebSocket. Start with
[WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md) if you haven't read it. What the server
sends back is in [websocket-events.md](./websocket-events.md).

Every request is a JSON text frame with a `type`. You can add `requestId`, your own label
for matching the reply. Many requests put their details in `metadata`.

## `send_message`

Sends a message to a chat you are in.

```json
{
  "type": "send_message",
  "chatId": "507f1f77bcf86cd799439011",
  "content": "Hello everyone!",
  "messageType": "text",
  "attachments": [],
  "mentions": ["<userId>"],
  "requestId": "r1"
}
```

| Field | Rule |
|---|---|
| `chatId` | Required. You must be a participant of the chat. |
| `content` | Required, even for image or file messages. Up to 4000 bytes. Emoji and non-English letters use more than one byte each, so the limit in characters can be lower (#51). |
| `messageType` | `text` (the default), `image`, `file`, `audio` or `video` |
| `attachments` | Up to 10. Each needs `url` and `originalName`, and `size` can be up to 50 MB. ECHO stores these details only. The file itself must be hosted somewhere else. |
| `mentions` | Up to 20 user IDs |
| `replyTo` | Accepted but ignored |

**You get:** a `response` with the new `messageId`. Everyone following the chat gets a
`message` event, including you. These two can arrive in either order.

**Errors:** `INVALID_REQUEST`, `INVALID_CHAT_ID`, `PERMISSION_DENIED`,
`PERMISSION_CHECK_FAILED`, `INVALID_CONTENT`, `MESSAGE_FAILED`, `CHANNEL_FULL`.

## `join_chat`

Despite the name, this opens a **direct chat** with another person. It finds your chat with
them, or creates one, and starts following it on this connection.

```json
{ "type": "join_chat", "senderId": "<the other person's user ID>", "requestId": "r2" }
```

- `senderId` is required. It is the *other* person, not you.
- `chatId` is ignored, so you can't use this request to follow a group (#48).

**You get:** a `join` event, and no `response`. Read the chat's ID from the event's
`chatId`.

**Errors:** `INVALID_USER_ID`, `CHAT_CREATION_FAILED`, `USER_NOT_AUTHENTICATED`.

## `leave_chat`

Stops live events for a chat on this connection. You stay a member of the chat, and you
follow it again the next time you connect.

```json
{ "type": "leave_chat", "chatId": "507f1f77bcf86cd799439011", "requestId": "r3" }
```

**You get:** no `response`. A `leave` event goes to the chat. You get `PERMISSION_DENIED` if
this connection isn't following the chat.

**Errors:** `INVALID_REQUEST`, `INVALID_CHAT_ID`, `PERMISSION_DENIED`, `LEAVE_FAILED`.

## `set_typing`

Tells the chat that you started or stopped typing.

```json
{
  "type": "set_typing",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "isTyping": true },
  "requestId": "r4"
}
```

**You get:** no `response`. A `typing` event goes to the chat. Each call writes to the
database (#40).

**Errors:** `INVALID_REQUEST`, `INVALID_CHAT_ID`, `PERMISSION_DENIED`, `TYPING_FAILED`.

## `mark_read`

Marks messages as read. IDs that aren't valid, or that aren't messages of this chat, are
skipped, but at least one must be left.

```json
{
  "type": "mark_read",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "messageIds": ["<messageId>", "<messageId>"] },
  "requestId": "r5"
}
```

**You get:** no `response`. A `delivery` event goes to the chat.

**Errors:** `INVALID_REQUEST`, `INVALID_CHAT_ID`, `PERMISSION_DENIED`, `MESSAGE_NOT_FOUND`,
`READ_FAILED`.

## `add_reaction` and `remove_reaction`

Adds or removes an emoji reaction. Both use the same format.

```json
{
  "type": "add_reaction",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "messageId": "<messageId>", "emoji": "👍" },
  "requestId": "r6"
}
```

`emoji` must be made of emoji characters only, up to 64 bytes. Every standard emoji fits,
including skin tones, flags and emoji joined from several, such as 👍🏽, 🇮🇳 or 👩‍💻 (the
longest is 35 bytes). Letters, spaces, line breaks, invisible characters, `.`, `$`, NUL and
invalid UTF-8 are refused with `INVALID_REQUEST`, and nothing changes. The server doesn't
check that the value is exactly one emoji, so `👍👍` is accepted.

**You get:** a `response` with `action` set to `reaction_added` or `reaction_removed`. A
`reaction` event goes to the chat. See #25.

**Errors:** `INVALID_REQUEST`, `INVALID_CHAT_ID`, `INVALID_MESSAGE_ID`,
`PERMISSION_DENIED`, `MESSAGE_NOT_FOUND`, `REACTION_FAILED`.

## `invite_user`

Adds people to a group. They are added right away as `pending` members, with no accept
step (#5). You must be the group's `owner` or an `admin`.

```json
{
  "type": "invite_user",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "userIds": ["<userId>", "<userId>"] },
  "requestId": "r7"
}
```

- IDs that aren't valid are skipped, but at least one must be valid.
- People already in the group are skipped, so they keep their role.
- If everyone is already in the group, you get `INVITE_FAILED`.

**You get:** a `response` with `action: "users_invited"`, `chatId` and `invitedUsers`. That
list holds every valid ID you sent, including people who were skipped. A `chat_updated`
event goes to the chat.

**Errors:** `INVALID_REQUEST`, `INVALID_CHAT_ID`, `PERMISSION_DENIED`, `INVITE_FAILED`.

## `remove_user`

Removes people from a group. You must be the group's `owner` or an `admin`.

```json
{
  "type": "remove_user",
  "chatId": "507f1f77bcf86cd799439011",
  "metadata": { "userIds": ["<userId>"] },
  "requestId": "r8"
}
```

- People not in the group, and you yourself, are skipped.
- Removed people lose their place in the group and the chat is taken off their chat list.
- Removed people keep getting live events until they reconnect (#14). See also #28.

**You get:** a `response` with `action: "users_removed"`, `chatId` and `removedUsers`. That
list holds every valid ID you sent, including people who were skipped. A `chat_updated`
event goes to the chat.

**Errors:** `INVALID_REQUEST` (also when nobody could be removed), `INVALID_CHAT_ID`,
`PERMISSION_DENIED`, `REMOVE_FAILED`.

## Not built yet

`edit_message`, `delete_message` and `update_chat` are recognized, but they only answer
with a `NOT_IMPLEMENTED` error.
