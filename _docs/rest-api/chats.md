# REST API: chats and messages

Index: [REST API](./README.md). Shared rules: [the basics](./basics.md).

## Chats

| Route | What it does |
|---|---|
| `POST chats/direct/:userId` | Opens a direct chat. If you already have one with this person, you get that one back. `400` if `userId` is you (#38). `403` `User is blocked` if either of you blocked the other, even when the chat exists (#27). `404` if either account no longer exists; the chat is not kept (#132). |
| `POST chats/group` | Creates a group. Send `name`, `participants` (user IDs, at least one valid) and optionally `description`. |
| `GET chats/hub/stats` | Live statistics, for operators only (see [below](#hub-statistics)) |

Each participant's `userInfo` (username, display name, avatar) is read when the reply is
built, so it is current; an account that no longer exists shows `displayName` "Deleted
user" (#36). There is no route that lists your chats, and none that renames a group.

## Messages

`GET messages/:chatID/messages` loads a chat's old messages, newest first.

| Option | Meaning |
|---|---|
| `limit` | Messages per page, 1 to 100. Default 50. Other values fall back to 50. |
| `before` | Message ID to start from. Leave it out for the newest page. |
| `afterSeq` | Catch up: messages with a `seq` above this, oldest first. Not with `before`. |

The reply's `data` has `chat`, `messages` and `pagination`:

```json
{ "limit": 50, "count": 50, "hasMore": true, "nextCursor": "<messageId>" }
```

**To load older messages,** send `nextCursor` as `before`. On the last page, `nextCursor`
is `null` and `hasMore` is `false`. Pages stay correct while new messages arrive. The old
`offset` option now returns `400`.

### Catching up on missed messages

Each message has `seq`: the chat's messages are numbered 1, 2, 3 in the order the server
saved them (#43). Live delivery can drop a message (a reconnect, a full send queue), so:

- Keep the highest `seq` you have per chat. On connect, and whenever a live `message`
  skips a number, send `afterSeq=<that seq>`. A chat's `stats.lastSeq` is its newest one.
- The reply is oldest first. While `hasMore` is `true`, send `pagination.nextAfterSeq` as
  the next `afterSeq`.
- A skip isn't always a lost message: a deleted account's messages leave gaps. Messages an
  older server copy saves during a deploy have no `seq` until the next start numbers them.

`afterSeq` that isn't a whole number from 0 up, or comes with `before`, gets `400`.

`reactions` maps emoji to `count`, `users` and `details` (ID to username, `""` before #75).
Messages saved before #72 have `reactions: null` until reacted to: treat it as none.

The names in a message (`sender`, reactions' `details`, mentions' `userNames`) are read when
the page is built, not kept from when it was sent, so a profile change shows on old
messages too. A sender whose account is gone shows `displayName` "Deleted user" (#36).

Only participants can read a chat. Everyone else gets `404 Chat not found`.

## Hub statistics

`GET chats/hub/stats` shows what this one server is doing. It is for operators, not for
the app. Besides the login token, the request needs an `X-Stats-Token` header that matches
the server's `STATS_TOKEN` setting. Without it, or while `STATS_TOKEN` is unset, the reply
is `404 Not found`. The reply has counts only, never user or chat IDs.

```json
{
  "success": true,
  "message": "Hub statistics retrieved successfully",
  "data": {
    "instanceId": "instance-1a2b3c4d-1730000000",
    "totalClients": 150,
    "totalChats": 45,
    "totalUsers": 120,
    "cacheSize": 80,
    "subscribedChannels": 170,
    "timestamp": "2026-11-04T12:00:00Z"
  }
}
```
