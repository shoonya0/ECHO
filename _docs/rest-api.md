# ECHO: REST API

Use REST for things that happen once: logging in, managing contacts, creating chats and
loading old messages. For live chat, use the [WebSocket](./WEBSOCKET_PROTOCOL.md).

## The basics

**Base path:** `/echo/v1/`. Locally, that is `http://localhost:8080/echo/v1/`.

**Login:** every route needs `Authorization: Bearer <token>`, except `signup`, `login` and
`/health`. You get the token from `login`.

**Replies** have this shape:

```json
{ "success": true, "message": "Login successful", "data": {} }
{ "success": false, "message": "Chat not found" }
```

Some failures add an `error` field with more detail. It never holds internal error text
(#41). For a body that fails validation it names each field by its JSON path, with the
rule: `"email: required; password: min=8; settings.name: required"`. A field of the wrong
type gives `"age: wrong type"`, and a body that isn't JSON gives `"request body is not
valid JSON for this endpoint"`. A bad ID in the path has no `error` field; `message` says
which ID is wrong.

**Token problems** return `401` in a different shape:

```json
{ "error": "token has been revoked", "code": "TOKEN_REVOKED" }
```

The codes are `MISSING_AUTH_HEADER`, `INVALID_AUTH_FORMAT`, `EMPTY_TOKEN`,
`TOKEN_VERIFICATION_FAILED` and `TOKEN_REVOKED`. The reply never says why a token failed
to verify.

Every request also checks the account itself, so a token stops working when its account
changes after login, in the same shape: `403` with `ACCOUNT_BANNED` or `ACCOUNT_INACTIVE`
(as `login` answers for those accounts), and `401` with `ACCOUNT_NOT_FOUND` when the
profile was deleted. A token must also have an ID (`jti`); every token `login` issues
does. The check is cached for up to 30 seconds,
so a ban takes effect within that time. Deleting your profile takes effect at once. If
the server can't check the token or the account, it returns `503` with
`AUTH_UNAVAILABLE`. That isn't a reason to log out: retry later.

| Status | Meaning |
|---|---|
| `200` | It worked. Creating something also returns `200`. |
| `400` | Something in your request is wrong |
| `401` | Token missing or not valid, or wrong password |
| `403` | You aren't allowed to do this |
| `404` | Not found, or you aren't allowed to know it exists |
| `409` | Conflict, such as "already in your contacts" |
| `500` | Something went wrong on the server |
| `503` | The server can't check your token right now (`AUTH_UNAVAILABLE`); retry later |

## Accounts

| Route | What it does |
|---|---|
| `POST signup` | Creates an account. Send `email`, `username` and `password` (8 characters or more). `409` if the email is taken. The server picks the new user's ID and returns it as `data._id`. An `_id` in the request is ignored. |
| `POST login` | Send `email` and `password`. Returns `data.token` (valid for 7 days) and `data.user`. `401` for a wrong password, `403` for a banned or inactive account. |
| `POST auth/logout` | Cancels the token you are using |
| `GET /health` | Returns `{"status":"ok"}`. It sits at the root, not under `/echo/v1/`, and needs no login. |

## Profile

| Route | What it does |
|---|---|
| `GET profile/` | Your profile |
| `PUT profile/` | Updates your profile. Only your username, email, phone, profile, presence and settings fields can change. Anything else is ignored. |
| `DELETE profile/delete` | Deletes your profile (see #35) |

## Finding people

| Route | What it does |
|---|---|
| `GET users/suggestions` | People you might add. Options: `page` (1 to 10000, default 1, clamped; #131), `limit` (default 20, max 50), `excludeIds` (comma-separated IDs). The reply has `data.items` and `data.pagination` (`limit`, `total`, `total_pages`). |
| `GET users/:id` | One person's public profile |
| `GET users/nearby` | Placeholder: returns success with no data |
| `GET users/popular` | Placeholder: returns success with no data |

## Contacts

All of these start with `users/contacts`.

| Route | What it does |
|---|---|
| `GET /` | Your contacts, favorites and pending requests both ways. People you blocked aren't in it; `GET /blocked` lists them (#62). |
| `GET /requests` | Requests sent to you |
| `GET /sent-requests` | Requests you sent |
| `GET /blocked` | People you blocked |
| `GET /favorites` | Your favorites |
| `POST /:targetUserId` | Sends a contact request. `400` if the target is you (#38). |
| `PUT /:requestId?action=accepted` | Accepts a request. Use `action=declined` to decline. |
| `DELETE /:contactId` | Removes a contact |
| `POST /blockUnblock/:userId` | Blocks or unblocks someone (`action=block` or `action=unblock`; block is the default). Only the person who blocked can unblock. `409` if already blocked, or if you didn't block them. The block never appears in the blocked person's lists or suggestions, but their contact requests to you fail with `403`. |
| `POST /favorite/:userId` | Adds to favorites |
| `DELETE /favorite/:userId` | Removes from favorites |

The five lists come in pages sorted by username, ignoring case (#39): `limit` 1 to 100
(default 10; below 1 or not a number counts as 10, over 100 as 100) and `page` 1 to 10000
(default 1, clamped). The reply has `data.items` and `data.pagination` (`limit`, `total`, `total_pages`).

## Chats

| Route | What it does |
|---|---|
| `POST chats/direct/:userId` | Opens a direct chat. If you already have one with this person, you get that one back. `400` if `userId` is you (#38). `403` `User is blocked` if either of you blocked the other, even when the chat exists (#27). |
| `POST chats/group` | Creates a group. Send `name`, `participants` (user IDs, at least one valid) and optionally `description`. |
| `GET chats/hub/stats` | Live statistics, for operators only (see [below](#hub-statistics)) |

There is no route that lists your chats, and none that renames a group.

## Messages

`GET messages/:chatID/messages` loads a chat's old messages, newest first.

| Option | Meaning |
|---|---|
| `limit` | Messages per page, 1 to 100. Default 50. Other values fall back to 50. |
| `before` | Message ID to start from. Leave it out for the newest page. |

The reply's `data` has `chat`, `messages` and `pagination`:

```json
{ "limit": 50, "count": 50, "hasMore": true, "nextCursor": "<messageId>" }
```

**To load older messages,** send `nextCursor` as `before`. On the last page, `nextCursor`
is `null` and `hasMore` is `false`. Pages stay correct while new messages arrive. The old
`offset` option now returns `400`.

`reactions` maps emoji to `count`, `users` and `details` (ID to username, `""` before #75).
Messages saved before #72 have `reactions: null` until reacted to: treat it as none.

Only participants can read a chat. Everyone else gets `404 Chat not found`.

## Groups

| Route | What it does |
|---|---|
| `POST groups/:groupID/add-members` | Adds people. Send `{ "userIDs": ["..."] }`. Owners and admins only. A group holds at most 100 people: going over (here, on create or on join) is `409` `Group is full` and adds nobody (#68). |
| `POST groups/:groupID/invites` | Creates an invite code and returns it in `data`, shaped like a list entry (`inviteCode`, `expiredDate`) (#104) |
| `GET groups/:groupID/invites` | Lists a group's invite codes |
| `GET groups/invites` | Lists invites sent to you |
| `DELETE groups/invites/:inviteID` | Deletes an invite code |
| `GET groups/invites/:inviteID/joined` | Shows who joined with that code |
| `POST groups/invites/:inviteID/:userID/send` | Sends a code to someone |
| `POST groups/join/:inviteCode` | Joins a group with a code, or accepts if you are a `pending` member (#103). Clears your invitations to the group. |

`:inviteID` and `:inviteCode` are both the code itself. Creating, listing, deleting and
sending codes, and `joined`, are for owners and admins (#30): members `403`, others `404`.

**How group membership works today**

- The creator becomes the `owner`, with status `accepted`. Creating a group also creates
  one invite code.
- People added when the group is created, or later with `add-members` or `invite_user`,
  become `member` with status `pending` and can read and write.
- Pending members have full access (#5) but no live messages (#17) until they accept by
  joining with any usable code (#103; `send` works for them). No decline yet.
- `add-members` returns `403` for members who aren't an owner or admin, `404` if you aren't
  in the group (#59). People already in are skipped; if everyone is already in, `409`.
- An invite code is 32 random hex characters (#15); codes made before 2026-11-14 are
  longer and work until they expire. Treat a code as an opaque string.
- Invite codes last 24 hours. Joining with a code makes you an `accepted` member. The code
  must be stored on that group, not deleted and not expired, or joining returns
  `400 Invalid invite`. Joining fails with `409` if you are already in the group, even as
  `pending`. If an earlier join failed halfway, retrying it finishes the join and returns
  `409`.
- Every invite route returns `400 Invalid invite` for a code that isn't stored on any
  group, and `404` for a group that doesn't exist or you aren't in. The list includes
  deleted and expired codes (`deleted`, `expiredDate`). Sending returns `400` for a deleted
  or expired code (#91), `404` for an unknown user and `409` for a member; sending again
  adds nothing. Joining removes your invitation; pending members, who can't join, keep it.

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
