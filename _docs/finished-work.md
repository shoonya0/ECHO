# ECHO: finished work

Older entries moved here from
[current-state.md](./current-state.md#recently-finished), newest first. Links point to
the docs that describe each change today. Newer work is listed on that page.

- **#40, login and signup (PR #139):** login attempts are counted per IP + account (10 in
  15 min) and per account (100 an hour), before bcrypt, by account ID, and cleared on
  success; 50 logins per IP in 10 min, 10 signups per IP an hour. Over that: `429`
  `RATE_LIMITED` with `Retry-After` ([REST](./rest-api/users.md#accounts),
  [decided](./decisions/accounts-and-privacy.md#how-login-and-signup-limits-find-the-client-40),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)). The client IP
  comes from `CLIENT_IP_HEADER`; `X-Forwarded-For` is no longer trusted. Code and security
  reviews fixed; left: case-sensitive emails (#36), ~2x at a window's edge.
- **#79 (PR #138):** `Hub.Restart` and the WebSocket controller's cancelled-context check
  are removed: nothing cancels the hub in production, and the path started duplicate loops
  and raced on `hub.Ctx`. Each background loop gets its context from `startLoops`.
- **#50 (PR #137):** `message` events carry `data.sender` and `data.mentions` from the
  saved message; `join` and `leave` carry the username and avatar the connection loaded
  at connect ([events](./websocket-events.md)). A rename shows on the next connect.
- **#48 (PR #136):** `join_chat` with `chatId` follows any chat you are in (block rule
  included), `senderId` still opens a direct chat, and both answer with a `response`
  holding `chatId`. A repeated join isn't announced again
  ([requests](./websocket-requests.md#join_chat)).
- **#132 (PR #135):** a new direct chat is kept only when both users got their `chats`
  entry (else `404` / `INVALID_USER_ID`, chat and entries removed), and opening an existing
  one writes back a missing entry ([REST](./rest-api/chats.md#chats)).
- **#131 (PR #134):** `GET users/suggestions` takes `page` 1 to 10000 (clamped, shared
  `services.MaxPage` with the contact lists), so a huge page no longer overflows the skip
  into a `500` ([REST](./rest-api/users.md#finding-people)).
- **#51 (PR #133):** a message `content` is limited to 4000 characters (code points), not
  bytes, so Hindi or emoji text is no longer cut at about a third
  ([decided](./decisions/chats-and-groups.md#how-the-message-length-limit-counts-51),
  [app changes](../Android/_docs/08_SERVER_CHANGES/messages.md)).
- **#123 + #124 (PR #127):** a join or registration releases the chat channel again after
  subscribing if a removal emptied the room meanwhile, and no `join` is announced for a
  member removed during the join. The release test hooks are `PubSubManager` fields
  ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)). Not done from the
  code review: a removal before the room entry (#117), one per-chat reconcile step.
- **CI race (PR #146):** a #22 test returned before its slow registration finished, which
  then raced the next test on `chatLoadTimeout`; it failed `main` after #145, now fixed.
- **#22 (PR #144):** the hub goroutine only changes maps. Each connection registers in its
  own goroutine (32 at most, chats read within 5 s); one that can't be set up closes with
  `1011` so the app reconnects. Disconnects release Redis channels in the background, a
  user is announced online once and offline only when no connection is left or queued
  ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)). Code review:
  9 findings, 8 fixed; a live burst check found a presence flood, fixed.
- **#40, typing part (PR #126):** `set_typing` only broadcasts the `typing` event; it no
  longer writes `typingUsers` to MongoDB or starts a timer that cleared users still typing
  ([requests](./websocket-requests.md#set_typing),
  [app changes](../Android/_docs/08_SERVER_CHANGES/auth-and-connection.md)).

Older work, from #122 (PR #125) and #49 (PR #130) back: [finished-work.md](./finished-work.md).
- **#122 (PR #125):** the server reads a connection's requests only after the hub has
  registered it (at most 10 s), so a `join_chat` sent right after connecting is no longer
  dropped ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)).
- **#54 (PR #121):** a disconnect the hub handles before the connection's registration
  closes its send queue and drops the presence it stored; the registration then skips the
  closed connection ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)).
  From the code review: #122, #123, #124 and a comment on #22.
- **#20 + #55 (PR #120):** rooms are tracked per connection (`ClientChats`). Every room
  removal (disconnect, `leave_chat`, `remove_user`) releases the chat channels left
  unused, a user's last connection releases `user:<id>`, and a join racing a release keeps
  its subscription ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting)).
- **#116 (PR #119):** a user's last connection closing leaves every chat and unsubscribes
  each of their chat channels (the loop used to skip some).
- **#14 (PR #118):** a member removed with `remove_user` gets the `chat_updated` and then
  no more live events from the group, on every server: a hub-internal message on the
  chat's Redis channel takes their connections out of the room
  ([events](./websocket-events.md), [app changes](../Android/_docs/08_SERVER_CHANGES/groups-and-invites.md)).
  The Android log is split into topic pages. Security review: nothing found; code review: #116, #117.
- **#49 (PR #130):** a chat a user gains while connected (a new direct chat, either side; a
  group they create; a code join, also a retry or a pending accept) reaches their open
  connections at once, through a hub-internal `internal.chat_added` on `user:<id>`
  ([pub/sub](./PUBSUB_ARCHITECTURE.md#connecting-and-disconnecting),
  [app changes](../Android/_docs/08_SERVER_CHANGES/groups-and-invites.md)). Not done from
  the review: a connection registering in the same ms misses the add; a removal racing an
  add is undone by the #117 re-check within 30 s.
- **#117 (PR #128):** every 30 s each server reads the participants of its rooms' chats
  (`services.ChatMembers`, keys only) and takes out connections of users who aren't
  members any more ([pub/sub](./PUBSUB_ARCHITECTURE.md#background-jobs)). Not done from the
  code review: a member removed and re-added within one pass stays out until they
  reconnect (#49); evicted connections get no event.
- **#106 (PR #115):** `send_message` and `invite_user` read the chat once, in the same
  query that checks membership (`services.GetChatForMember`); replies are unchanged.
  `remove_user` still reads it twice.
- **#112 (PR #114):** creating a group checks the write that adds it to the creator's chat
  list; if that write fails or finds no user, the new group is removed and the request
  fails (`404` or `500`) before any invitation goes out.
- **#103 (PR #113):** a `pending` group member accepts by joining with any usable code of
  the group; `send` works for pending members, and joining clears every invitation to the
  group. Live messages start at the next connection ([REST](./rest-api/groups.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). A dedicated accept and decline
  action is left to #5 ([decisions](./decisions/chats-and-groups.md#how-a-pending-member-accepts-103-5)).
- **#68 (PR #111):** a group holds at most `settings.maxParticipants` (default 100). Create,
  add and join past it add nobody: REST `409`, WebSocket `GROUP_FULL`, checked in the
  update filter ([REST](./rest-api/groups.md#groups), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  From the code review: #112.
- **#16 (PR #109):** a group's `stats.participantCount` follows every add, join and
  removal, and a startup backfill resets drifted groups (skipping malformed ones). It
  counts pending members and deleted accounts ([app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  From the code review: #110 (derive the count instead of maintaining it).
- **#62 (PR #108):** `GET users/contacts` leaves out people you blocked; `GET /blocked`
  lists them ([REST](./rest-api/users.md#contacts), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
- **#104 (PR #107):** creating an invite code returns it in `data`, shaped like an entry
  of the group's list ([REST](./rest-api/groups.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
- **#59 (PR #105):** a non-member gets the same answer whether or not a chat exists:
  `PERMISSION_DENIED` from `send_message`, `invite_user` and `remove_user`, `404` from
  REST `add-members` ([protocol](./WEBSOCKET_PROTOCOL.md#error-codes),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). Security review: nothing found.
  From the code review: #106.
- **#30 (PR #102):** only owners and admins create, list, delete or send invite codes or
  see who joined: other members get `403`, anyone not in the group `404`
  ([REST](./rest-api/groups.md#groups), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
  Security review: nothing found. From the code review: #103, #104.
- **#39 (PR #129):** the five contact lists come in pages: `page` 1 to 10000, cut to
  `limit`, sorted by username ignoring case, with the real `total`
  ([REST](./rest-api/users.md#contacts), [app changes](../Android/_docs/08_SERVER_CHANGES/contacts-and-blocking.md)).
  Not done from the review: offset pages can shift when the list changes; `users/suggestions`
  has the same unbounded `page`.
- **#67 (PR #101):** the contract test pages history with `?before=<nextCursor>` and checks
  that `offset` gets `400`; every step passes against a local server.
- **#60 (PR #100):** a `chatId` in any case is lowercased where WebSocket requests come
  in, so its events reach the chat ([requests](./websocket-requests.md)).
- **#47 (PR #99):** `send_message` replies have a top-level `requestId` ([events](./websocket-events.md)).
- **#75 (PR #98):** reaction `details` hold usernames; older entries keep `""`
  ([REST](./rest-api/chats.md#messages), [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)).
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
  stored gets `400` on every route ([REST](./rest-api/groups.md#groups),
  [app changes](../Android/_docs/08_SERVER_CHANGES/README.md)). Security review: nothing found.
  The code review found #91 (a deleted code can still be sent).
- **#83 (PR #87):** `invite_user` answers a failed chat lookup (timeout, MongoDB down) with
  the retryable `INVITE_FAILED`; only a chat that doesn't exist is `INVALID_CHAT_ID`.
  `GetChat` now returns `ErrChatNotFound` for a missing chat.
- **#39 part 1 (PR #85):** `limit=0` no longer crashes the contact lists with a `500`;
  `limit` is kept in 1..100 ([REST](./rest-api/users.md#contacts)). Still open on #39: the lists
  aren't cut to `limit`, which needs a cursor first.
- **#41 (PR #81):** no REST or WebSocket reply carries internal error text. A `400`'s
  `error` names each field by its JSON path and rule (`password: min=8`), and
  `invite_user` answers with specific codes ([REST](./rest-api/basics.md#the-basics),
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
  ([rest-api/](./rest-api/chats.md#messages)). The live check found #75.
