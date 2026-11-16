# Server changes: messages and replies

Part of [server changes the app must handle](./README.md).

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
