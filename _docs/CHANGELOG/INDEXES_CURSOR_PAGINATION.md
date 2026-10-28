# MongoDB Indexes and Cursor Pagination

## Indexes (`internal/database/indexes.go`)

`database.EnsureIndexes` runs in the background at server startup, right after the
MongoDB connection, with a 10-minute timeout, so a long first build doesn't delay the
server from listening. Creating an index that already exists is a no-op, so it is safe
on every start.

| Collection | Index | Serves |
|---|---|---|
| `messages` | `{chatId: 1, _id: -1}` | `GET /messages/:chatID/messages` |
| `users` | `{email: 1}`, unique | Login lookup, signup duplicate check |

- A failure is logged, not fatal, and doesn't stop the other collections' indexes. The
  usual cause is existing duplicate emails, which block the unique index: remove the
  duplicates and restart.
- With the unique index, two concurrent signups for the same email no longer both
  succeed: the second gets `409 email already registered`.
- No index for the user suggestions sort: that `$sort` runs inside a `$lookup`
  sub-pipeline after an `$expr` match, where an index is unlikely to be used.
- `chats` has no secondary index: its queries filter on `_id` or on
  `participants.<userID>`, a per-user key a fixed index cannot cover.

## Cursor pagination for message history

**Breaking change** to `GET /echo/v1/messages/:chatID/messages`.

| | Before | After |
|---|---|---|
| Query | `?limit=50&offset=100` (now `400`) | `?limit=50&before=<messageId>` |
| `pagination` | `{ limit, offset, count, totalCount, page, totalPages }` | `{ limit, count, hasMore, nextCursor }` |

- Messages come newest first. Omit `before` for the newest page; to load older
  messages, send the previous response's `nextCursor` as `before`. `nextCursor` is
  `null` (and `hasMore` is `false`) on the last page.
- An invalid `before` returns `400 Invalid before cursor`. Sending `offset` returns `400` with a pointer to `before`,
  so old clients fail loudly instead of reloading the newest page forever.
- `totalCount`, `page` and `totalPages` were removed. They were computed from the page
  length rather than the chat, so they were always wrong.

**Why:** `skip` made MongoDB read and discard every skipped message, so deep pages got
slower, and pages shifted (duplicating or missing messages) whenever new messages
arrived. A `_id < before` range on the new index reads only `limit + 1` documents.

Client docs updated: `Android/_docs/02_UI_SCREENS.md`, `03_DATA_MODELS.md`,
`04_API_INTEGRATION.md`.
