# Server changes: messages and replies

Part of [server changes the app must handle](./README.md).

## Messages have a per-chat number: catch up on missed ones (#43)

- Every message in history and in live `message` events has `seq` (1, 2, 3 per chat).
  Chats have `stats.lastSeq`. Store the highest `seq` seen per chat.
- On every (re)connect, and when a live `seq` skips a number, call
  `GET messages/:chatID/messages?afterSeq=<highest>` and page with `nextAfterSeq` while
  `hasMore`. Merge by message ID: you may get one you already have.
- A gap can be a deleted account's messages, not a loss; the catch-up call settles it.

Details: [chats.md](../../../_docs/rest-api/chats.md#catching-up-on-missed-messages).

## Live events name the sender (#50)

- A `message` event now has `data.sender` (`userId`, `username`, `displayName`, `avatar`,
  the same object as REST messages) and `data.mentions` (user IDs). Show it directly;
  no user lookup is needed for an unknown sender.
- `join` and `leave` events now fill `data.username` and `data.avatar` (left out when
  empty), as the user's profile was when that connection opened
  ([details](../../../_docs/websocket-events.md)).

## The 4000 limit counts characters (#51)

- `send_message` `content` may hold 4000 characters (Unicode code points), not 4000
  bytes. Before, text in Hindi or emoji was refused at about 1300 to 2000 characters.
- Count the same way in the message box: Kotlin `text.codePointCount(0, text.length)`,
  not `text.length` (UTF-16 units count an emoji as 2). Over the limit the server answers
  `INVALID_CONTENT` ([details](../../../_docs/websocket-requests.md)).

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

Details: [websocket-requests.md](../../../_docs/websocket-requests.md#add_reaction-and-remove_reaction).

## `send_message` replies have a top-level `requestId` (#47)

- The `response` to `send_message` now has `requestId` at the top level, like every other
  `response` and `error`. Before, it was only in `data.requestId`, so the plan's
  `frame.requestId ?: return` dropped every `send_message` reply and the send timed out.
- Match replies on the top-level `requestId`. `data.requestId` is still there.

Details: [websocket-events.md](../../../_docs/websocket-events.md).

## Mentions keep only chat participants (#40)

- `mentions` in a `send_message` request keeps only users who are participants of the
  chat, each once, in the order sent. Other IDs are dropped without an error, so the
  `message` event and history can hold fewer mentions than were sent.
- In history, `mentions.userIds` and `mentions.userNames` now line up one to one.

Details: [websocket-requests.md](../../../_docs/websocket-requests.md#send_message).

## Old messages show current names (#36)

- `GET messages/:chatID/messages` fills `sender`, reactions' `details` and mentions'
  `userNames` with each user's current name and avatar. After someone changes their
  profile, reload history to show it; don't cache names per message forever.
- A sender whose account was deleted has `displayName` "Deleted user" and an empty
  `username` and `avatar`.
- `participants.<id>.userInfo` in `POST chats/direct/:userId` and `POST chats/group`
  replies is current too.
- Live `chat` events carry the name from when the message was sent. `typing`, `join`,
  `leave`, invite and removal events show the current name within seconds of a change.

Details: [chats.md](../../../_docs/rest-api/chats.md#messages).
