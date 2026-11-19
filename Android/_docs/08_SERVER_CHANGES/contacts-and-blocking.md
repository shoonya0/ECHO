# Server changes: contacts, blocking and yourself as a target

Part of [server changes the app must handle](./README.md).

## Contact lists come in pages (#39)

- The five lists (`users/contacts`, `/requests`, `/sent-requests`, `/blocked`,
  `/favorites`) now return at most `limit` entries (1 to 100, default 10), sorted by
  username ignoring case. Before, they returned every entry whatever `limit` said.
- Ask for more with `page` (from 1). `pagination.total` is the real count and
  `total_pages` the number of pages. Load pages until `page` reaches `total_pages`, or a
  user with more than `limit` contacts sees only the first ones
  ([details](../../../_docs/rest-api/users.md#contacts)).

## Blocked people aren't in the contacts list (#62)

- `GET users/contacts` no longer lists people you blocked: show them from
  `GET users/contacts/blocked`, and drop them from cached contacts after a block
  ([details](../../../_docs/rest-api/users.md#contacts)).

## Yourself as a target (#38)

- A contact request to yourself, or `POST chats/direct/<your ID>`, now gets `400`. Before,
  it went through and stored broken data. `join_chat` with your own `senderId` gets
  `INVALID_USER_ID`. Don't offer "Add contact" or "Message" on your own profile.

Details: [rest-api/](../../../_docs/rest-api/users.md#contacts).

## Blocks apply in direct chats (#27)

- If either person blocked the other, `POST chats/direct/<ID>` gets `403` `User is
  blocked`, and `join_chat` gets `PERMISSION_DENIED` with the same message. This holds
  for a chat that already exists.
- `send_message`, `set_typing`, `mark_read`, `add_reaction` and `remove_reaction` in
  that direct chat get `PERMISSION_DENIED` `User is blocked`, for both the blocker and
  the blocked person. Nothing is stored or delivered. Don't send typing or read events
  for a blocked chat.
- The chat and its history stay in both chat lists. Show it as read-only with a "blocked"
  note instead of an error, and hide the message box. Unblocking makes it work again.
- Group chats aren't affected.

Details: [websocket-requests.md](../../../_docs/websocket-requests.md).

## Privacy settings now apply (#32)

- `settings.privacy.showOnlineStatus`: `contacts` shows you online only to accepted
  contacts, `none` to nobody, in contact lists and in `presence` events. A contact who
  hides from you looks `offline`, with no `lastSeen`.
- `showLastSeen`: `false` hides `lastSeen` (and `lastActivity`). `presence` events then have
  no `lastSeen` key: handle it missing. It is now on for everyone; offer it as a switch.
- `allowContactBy`: `POST users/contacts/:id` answers `403` when the target accepts
  requests from nobody, or only from people sharing a group with them. Tell the user the
  person doesn't accept requests.
- `PUT profile/` answers `400` for a `showOnlineStatus` or `allowContactBy` other than
  `everyone`, `contacts` or `none`. It now changes only the fields you send: before, sending
  one setting reset the other settings (and presence fields) in that group to `""`/`false`.
- A privacy change takes effect for live events at the next connect; reconnect after saving.

Details: [users.md](../../../_docs/rest-api/users.md#contacts),
[decided](../../../_docs/decisions/accounts-and-privacy.md#how-privacy-settings-apply-32).
