# WebSocket events

Everything the server sends over the WebSocket. Start with
[WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md) if you haven't read it. What an app can
send is in [websocket-requests.md](./websocket-requests.md).

In the examples below, `timestamp` is left out to keep them short. Every event has one.

## `response`: your request worked

The `response` comes in three shapes, so read them carefully.

**Welcome.** Sent once, right after you connect:

```json
{
  "type": "response",
  "userId": "<your userId>",
  "data": { "status": "connected", "clientId": "<uuid>", "instanceId": "<server>" }
}
```

**Reply to `send_message`:**

```json
{
  "type": "response",
  "data": {
    "success": true,
    "messageId": "<messageId>",
    "data": {
      "messageId": "<messageId>",
      "timestamp": "2026-11-04T12:00:00Z",
      "chatId": "<chatId>",
      "messageType": "text"
    },
    "requestId": "r1"
  },
  "requestId": "r1"
}
```

**Reply to `join_chat` (`data.data.chatId`), reactions, `invite_user` and `remove_user`:**

```json
{
  "type": "response",
  "data": {
    "success": true,
    "data": { "action": "reaction_added", "messageId": "<messageId>", "emoji": "👍", "changed": true },
    "requestId": "r6"
  },
  "requestId": "r6"
}
```

**To match a reply to its request, read the top-level `requestId`.** Every `response` and
`error` has it; a `response` also has it in `data.requestId` (#47). The details of the
reply are in `data.data`.

## `error`: your request failed

```json
{
  "type": "error",
  "data": { "code": "PERMISSION_DENIED", "message": "You don't have permission..." },
  "requestId": "r1"
}
```

Here `requestId` is at the top level. It is missing for `PARSE_ERROR`, because the server
couldn't read your message. All codes are listed in
[WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md#error-codes).

## `message`: someone sent a message

```json
{
  "type": "message",
  "chatId": "<chatId>",
  "userId": "<sender's userId>",
  "username": "<sender's username>", "avatar": "<sender's avatar URL>",
  "data": {
    "id": "<messageId>",
    "chatId": "<chatId>",
    "senderId": "<sender's userId>",
    "sender": { "userId": "...", "username": "...", "displayName": "...", "avatar": "..." },
    "content": "Hello everyone!", "messageType": "text", "mentions": ["<mentioned userId>"],
    "createdAt": "2026-11-04T12:00:00Z"
  }
}
```

- `sender` is the object messages loaded [over REST](./rest-api/chats.md#messages) have, with
  the sender's name and avatar when it was sent (#50); history loads the current ones (#36). `attachments` and `mentions` (user IDs) appear only when there
  are some, and so does the envelope `avatar`. You also get this event for your own messages.

## `typing`: someone is typing

```json
{
  "type": "typing",
  "chatId": "<chatId>",
  "userId": "<userId>",
  "data": {
    "userId": "<userId>",
    "username": "some_user",
    "isTyping": true,
    "timestamp": "2026-11-04T12:00:00Z"
  }
}
```

## `delivery`: someone read messages

```json
{
  "type": "delivery",
  "chatId": "<chatId>",
  "userId": "<reader's userId>",
  "data": {
    "messageIds": ["<messageId>"],
    "status": "read",
    "timestamp": "2026-11-04T12:00:00Z",
    "instanceID": "<server>"
  }
}
```

`status` is always `read` today. Ignore `instanceID`: it is for the server's own tracking.

## `reaction`: a reaction changed

```json
{
  "type": "reaction",
  "chatId": "<chatId>",
  "userId": "<userId>",
  "data": {
    "messageId": "<messageId>",
    "userId": "<userId>",
    "username": "some_user",
    "emoji": "👍",
    "action": "add",
    "timestamp": "2026-11-04T12:00:00Z"
  }
}
```

`action` is `add` or `remove`. It is only sent when the reaction really changed, so the
app can add or take away one for `userId` without checking (#25).

## `join` and `leave`: someone started or stopped following

```json
{
  "type": "join",
  "chatId": "<chatId>",
  "userId": "<userId>",
  "username": "<username>",
  "data": { "userId": "<userId>", "username": "<username>", "avatar": "<URL>", "action": "join", "timestamp": "..." }
}
```

These are about live connections, not group membership. `username` and `avatar` are the
current profile, up to 5 minutes old (#50, #36); `avatar` is left out when empty.

## `presence`: someone's online status changed

```json
{
  "type": "presence",
  "userId": "<userId>",
  "data": { "userId": "<userId>", "status": "online", "lastSeen": "..." }
}
```

No `chatId`. Once to each connection sharing a chat (only contacts, or nobody, per `showOnlineStatus`): the chosen status at the
first connection on a server (maybe twice), `offline` when the last closes. No `lastSeen` if hidden (#31, #32).

## `chat_updated`: group members changed

Sent to the chat after `invite_user` or `remove_user`. The event doesn't say who was added
or removed: the `inviteeId`, `inviteeName`, `removedId` and `removedUser` fields are always
empty. Only the caller's `response` lists the IDs.

```json
{ "chatId": "<chatId>", "inviterId": "<userId>", "inviterName": "some_user", "inviteeId": "", "inviteeName": "", "timestamp": "..." }
{ "chatId": "<chatId>", "removedById": "<userId>", "removedBy": "some_user", "removedId": "", "removedUser": "", "timestamp": "..." }
```

These are the two forms of `data`: the first after `invite_user`, the second after
`remove_user`. The removed members still get this event, and nothing from the chat after
it: their open connections leave the chat on every server (#14).

## Reserved, not sent yet

The code names these event types, but nothing sends them: `user_updated`,
`message_edited`, `message_deleted` and `notification`.
