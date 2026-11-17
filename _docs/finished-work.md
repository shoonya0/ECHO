# ECHO: finished work

Older entries moved here from
[current-state.md](./current-state.md#recently-finished), newest first. Links point to
the docs that describe each change today. Newer work is listed on that page.

- **#39 (PR #129):** the five contact lists come in pages: `page` 1 to 10000, cut to
  `limit`, sorted by username ignoring case, with the real `total`
  ([REST](./rest-api.md#contacts), [app changes](../Android/_docs/08_SERVER_CHANGES/contacts-and-blocking.md)).
  Not done from the review: offset pages can shift when the list changes; `users/suggestions`
  has the same unbounded `page`.
- **#67 (PR #101):** the contract test pages history with `?before=<nextCursor>` and checks
  that `offset` gets `400`; every step passes against a local server.
- **#60 (PR #100):** a `chatId` in any case is lowercased where WebSocket requests come
  in, so its events reach the chat ([requests](./websocket-requests.md)).
- **#47 (PR #99):** `send_message` replies have a top-level `requestId` ([events](./websocket-events.md)).
- **#75 (PR #98):** reaction `details` hold usernames; older entries keep `""`
  ([REST](./rest-api.md#messages), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
- **#25 (PR #97):** each user counts once per emoji; a no-op add or remove replies
  `changed: false` with no event, and `count` is set from `users`, healing drift
  ([requests](./websocket-requests.md#add_reaction-and-remove_reaction)). Not done from
  the review: odd old reaction shapes, a leftover `{count: 0}`, the no-op lookup.
- **#27 (PR #96):** a block either way closes the direct chat to both people. Opening it
  gets `403` / `PERMISSION_DENIED` `User is blocked`, and so do `send_message`,
  `set_typing`, `mark_read` and reactions; history still reads. Groups aren't affected
  ([requests](./websocket-requests.md), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  Not done from the code review: contact requests still miss a half-written block.
- **#38 (PR #95):** a contact request or direct chat with yourself gets `400`
  (`INVALID_USER_ID` from `join_chat`) instead of storing broken data
  ([app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
- **#28 (PR #94):** only the owner can remove the owner (also when the owner is only
  stored as `ownerId`); a request naming the owner removes nobody. Listing yourself no
  longer drops the group from your chat list, `removedUsers` lists only the people removed,
  and the role is re-checked in the write ([requests](./websocket-requests.md#remove_user),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). The code review's other points
  (a race reported as `PERMISSION_DENIED`, the second write not checked, #37) were left.
- **#15 (PR #90):** invite codes are 16 random bytes (32 hex characters) with nothing
  inside; the AES key that was in the repo is gone. The invite routes find the group by
  the stored code (indexed), so old codes work until they expire, and a code that isn't
  stored gets `400` on every route ([REST](./rest-api.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). Security review: nothing found.
  The code review found #91 (a deleted code can still be sent).
- **#83 (PR #87):** `invite_user` answers a failed chat lookup (timeout, MongoDB down) with
  the retryable `INVITE_FAILED`; only a chat that doesn't exist is `INVALID_CHAT_ID`.
  `GetChat` now returns `ErrChatNotFound` for a missing chat.
- **#39 part 1 (PR #85):** `limit=0` no longer crashes the contact lists with a `500`;
  `limit` is kept in 1..100 ([REST](./rest-api.md#contacts)). Still open on #39: the lists
  aren't cut to `limit`, which needs a cursor first.
- **#41 (PR #81):** no REST or WebSocket reply carries internal error text. A `400`'s
  `error` names each field by its JSON path and rule (`password: min=8`), and
  `invite_user` answers with specific codes ([REST](./rest-api.md#the-basics),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). Security review: nothing found.
- **#40 part 1 (PR #80):** a WebSocket frame over 64 KiB closes with `1009`, and each
  connection gets 20 requests a second (bursts of 40, pings included). A request over that
  isn't handled and gets `RATE_LIMITED` ([protocol](./WEBSOCKET_PROTOCOL.md#stay-connected),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). Security review: nothing found.
  Still open on #40: a per-user limit (one account can open many sockets), `set_typing`'s
  MongoDB write, mention lookups, `CheckOrigin`, and login/signup limits.
- **Render (2026-11-12):** `STATS_TOKEN` is set (the Blueprint generated it), and
  `GET chats/hub/stats` with `X-Stats-Token` returns `200`. The MongoDB password,
  `JWT_SECRET` and `STATS_TOKEN` were rotated after being exposed, so every login before
  that was signed out.
- **#76 (PR #78):** open WebSockets now
  close when their session ends. Logout closes the connections opened with that token
  (other devices stay connected); deleting the profile closes all of the user's. Both act
  at once on every server, through the Redis channel `control:sessions`. Every open
  connection is also re-checked every 30 seconds, so a ban made in MongoDB closes it within
  about a minute (52 to 58 s in the live checks), and a connection closes when its token
  expires. If a check can't be made, the pass stops and the connections stay open until
  the next one. The close frame says why: `4401` `TOKEN_REVOKED`,
  `TOKEN_EXPIRED` or `ACCOUNT_NOT_FOUND`, `4403` `ACCOUNT_BANNED` or `ACCOUNT_INACTIVE`
  ([WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md#stay-connected)). A closed connection's
  read loop finishes the close handshake within 5 seconds instead of at its 60-second
  deadline. The middleware and the re-check share one check (`services.CheckSession`). The
  code review found #79; the security review found nothing.
- **#72 (PR #74):** reactions work on real messages. New messages store `reactions: {}` instead of
  `null`, which MongoDB refused to create reaction fields in. A message that still has
  `null` (every message saved before the fix, or one written by an older server copy
  during a deploy) is repaired when someone first reacts to it, so there is no startup
  migration. Old messages nobody has reacted to still show `reactions: null` in history
  ([rest-api.md](./rest-api.md#messages)). The live check found #75.
