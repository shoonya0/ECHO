# REST API: groups

Index: [REST API](./README.md). Shared rules: [the basics](./basics.md).

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
