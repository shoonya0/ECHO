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

**Reply to `send_message`.** Here `requestId` is only inside `data` (#47):

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
  }
}
```

**Reply to reactions, `invite_user` and `remove_user`.** Here `requestId` is in both places:

```json
{
  "type": "response",
  "data": {
    "success": true,
    "data": { "action": "reaction_added", "messageId": "<messageId>", "emoji": "👍" },
    "requestId": "r6"
  },
  "requestId": "r6"
}
```

**To match a reply to its request, always read `data.requestId`.** It works for all of
them. The details of the reply are in `data.data`.

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
  "data": {
    "id": "<messageId>",
    "chatId": "<chatId>",
    "senderId": "<sender's userId>",
    "content": "Hello everyone!",
    "messageType": "text",
    "createdAt": "2026-11-04T12:00:00Z"
  }
}
```

- This event has no sender name, avatar or mentions (#50). Find the sender by `senderId`.
  Messages loaded [over REST](./rest-api.md#messages) include a `sender` object.
- `attachments` appears only when the message has some.
- You also get this event for your own messages.

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

`action` is `add` or `remove`.

## `join` and `leave`: someone started or stopped following

```json
{
  "type": "join",
  "chatId": "<chatId>",
  "userId": "<userId>",
  "data": { "userId": "<userId>", "username": "", "action": "join", "timestamp": "..." }
}
```

These are about live connections, not group membership. `username` is always empty today (#50).

## `presence`: someone's online status changed

```json
{
  "type": "presence",
  "userId": "<userId>",
  "data": { "userId": "<userId>", "status": "online", "lastSeen": "...", "instanceID": "<server>" }
}
```

This event has no `chatId`. Don't rely on it: "online" is rarely sent, and "offline" doesn't
reach chat members (#31).

## `chat_updated`: group members changed

Sent to the chat after `invite_user` or `remove_user`. The event doesn't say who was added
or removed: the `inviteeId`, `inviteeName`, `removedId` and `removedUser` fields are always
empty. Only the caller's `response` lists the IDs.

```json
{ "chatId": "<chatId>", "inviterId": "<userId>", "inviterName": "some_user", "inviteeId": "", "inviteeName": "", "timestamp": "..." }
{ "chatId": "<chatId>", "removedById": "<userId>", "removedBy": "some_user", "removedId": "", "removedUser": "", "timestamp": "..." }
```

These are the two forms of `data`: the first after `invite_user`, the second after
`remove_user`.

## Reserved, not sent yet

The code names these event types, but nothing sends them: `user_updated`,
`message_edited`, `message_deleted` and `notification`.
