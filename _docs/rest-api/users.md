# REST API: accounts, profile and contacts

Index: [REST API](./README.md). Shared rules: [the basics](./basics.md).

## Accounts

| Route | What it does |
|---|---|
| `POST signup` | Creates an account. Send `email`, `username` and `password` (8 characters or more). The email is stored trimmed and in lower case, so `409` if it is taken in any case (#36). The `username` is trimmed and must be 3 to 30 letters, digits, `_` or `.` (else `400`); it keeps its capitals but is unique ignoring them, so `409` "Username already taken" if another account has it in any case (#36). The server picks the new user's ID and returns it as `data._id`. An `_id` in the request is ignored. `429` after 10 signups from one IP in an hour (#40). |
| `POST login` | Send `email` and `password`. The email's case doesn't matter (#36). Returns `data.token` (valid for 7 days) and `data.user`. `401` for a wrong password, `403` for a banned or inactive account, with `code` `ACCOUNT_BANNED` or `ACCOUNT_INACTIVE` (#171). `429` after 10 attempts on one account from one IP in 15 minutes, 100 on one account from all IPs in an hour, or 50 logins from one IP in 10 minutes, even with the right password. A successful login clears the account's counts (#40). |
| `POST auth/logout` | Cancels the token you are using |
| `GET /health` | Returns `{"status":"ok"}`. It sits at the root, not under `/echo/v1/`, and needs no login. |

## Profile

| Route | What it does |
|---|---|
| `GET profile/` | Your profile |
| `PUT profile/` | Updates your profile. Only your username, phone, profile, presence and settings fields can change. Anything else is ignored, `email` included: it can't be changed yet ([decided](../decisions/accounts-and-privacy.md#changing-the-email-36)). `presence.isOnline` and `presence.lastSeen` are ignored too: the server sets them. `presence.status` other than `online`, `away`, `dnd`, `invisible` or `offline` (case matters) answers `400` (#31). A new `username` follows the signup rules (`400`, `409`); sending your current one back is always accepted, even one from before #36 that lacks the format, and changing only its capitals is allowed (#36). Only the fields you send change: before #32, sending one field of a group reset the group's others. |
| `DELETE profile/delete` | Deletes your account. You leave every group (an owner hands it on) and every contact list; your messages and direct chats stay for the others, shown as "Deleted user". Your token stops working at once (`401 ACCOUNT_NOT_FOUND`) (#35) |

`presence` in replies (your profile, `login`, contacts) is live (#31): `isOnline` says whether
the person is connected now. `status` is their choice (`online`, `away`, `dnd`, `invisible`)
while they are connected; to others it is `offline` when they aren't, or chose `invisible`,
and `lastSeen` is when their last connection closed. You see your own status as you set it.
`settings.privacy` narrows what others see (#32): `showOnlineStatus` (`everyone`,
`contacts`, `none`) and `showLastSeen`. [Decided](../decisions/accounts-and-privacy.md#how-privacy-settings-apply-32).

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
| `POST /:targetUserId` | Sends a contact request. `400` if the target is you (#38). `403` if the target's `allowContactBy` shuts you out: `none`, or `contacts` when you share no group as members (#32). |
| `PUT /:requestId?action=accepted` | Accepts a request. Use `action=declined` to decline. |
| `DELETE /:contactId` | Removes a contact |
| `POST /blockUnblock/:userId` | Blocks or unblocks someone (`action=block` or `action=unblock`; block is the default). Only the person who blocked can unblock. `409` if already blocked, or if you didn't block them. The block never appears in the blocked person's lists or suggestions, but their contact requests to you fail with `403`. |
| `POST /favorite/:userId` | Adds to favorites |
| `DELETE /favorite/:userId` | Removes from favorites |

The five lists come in pages sorted by username, ignoring case (#39): `limit` 1 to 100
(default 10; below 1 or not a number counts as 10, over 100 as 100) and `page` 1 to 10000
(default 1, clamped). The reply has `data.items` and `data.pagination` (`limit`, `total`, `total_pages`).
