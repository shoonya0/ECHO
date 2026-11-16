# Server changes: groups and invite codes

Part of [server changes the app must handle](./README.md).

## Groups: size limit, count, accepting (#68, #16, #103)

- A group holds at most 100 people. A create, add or join by code past that adds nobody:
  REST `409` `Group is full`, WebSocket `GROUP_FULL`. Show "group is full"; don't retry.
- `stats.participantCount` is now correct, but counts pending members (#5) and deleted accounts (#35).
- A `pending` member accepts by joining with a code (`200`, invites cleared); then reconnect for live messages.

Details: [rest-api.md](../../../_docs/rest-api.md#groups).

## Creating an invite code returns it (#104)

- `POST groups/:groupID/invites` now returns the new code in `data`, in the same shape as
  an entry of `GET groups/:groupID/invites` (`inviteCode`, `expiredDate`, `deleted`, ...).
  Share `data.inviteCode` directly; don't take the first entry of the list, which is the
  code made when the group was created.

Details: [rest-api.md](../../../_docs/rest-api.md#groups).

## Invite codes (#15)

- New invite codes are 32 random hex characters. Before, they were much longer and held the
  group ID and the creator's ID and name, encrypted with a key anyone could read. Store and
  show a code as an opaque string; don't parse it or assume its length.
- Codes made before the change keep working until they expire (24 hours).
- A code that isn't stored on any group now gets `400 Invalid invite` on every invite
  route, not `200` or `500`.
- Sending a deleted code to someone (`POST /groups/invites/:code/:userID/send`) now gets
  `400 Invalid invite` too, as an expired one already did (#91).

Details: [rest-api.md](../../../_docs/rest-api.md#groups).

## Only owners and admins manage invite codes (#30)

- Creating, listing, deleting and sending invite codes, and "who joined", now return `403`
  for members who aren't the owner or an admin, and `404` for anyone not in the group or a
  group that doesn't exist. Show
  those actions only to owners and admins. Joining with a code is unchanged.
- The list includes deleted and expired codes; check `deleted` and `expiredDate`.
- Sending a code returns `404` for an unknown user and `409` for someone already in the
  group (both were `500`). Sending it again doesn't add a second invitation, and joining
  removes the invitation from `GET groups/invites`. People added when the group was created
  are pending members: they can't join by code, so their invitation stays listed.

Details: [rest-api.md](../../../_docs/rest-api.md#groups).

## Removing group members (#28)

- `remove_user` refuses to remove the group's owner unless the owner sends it: the reply is
  `PERMISSION_DENIED` and nobody in the request is removed. Hide "Remove" on the owner for
  admins.
- If your role or the group changed while the request ran, the reply is also
  `PERMISSION_DENIED`. Reload the group before retrying.
- Listing yourself among the users no longer takes the group off your own chat list.
- The reply's `removedUsers` now lists only the people removed. Before, it echoed every ID
  sent, including you and people not in the group; update the member list from it.
- A removed member gets the `chat_updated` event and then no more live events from the
  group, on any open connection (#14). When it arrives, the app can't tell from the event
  alone who was removed: reload the group, and if it now fails, take it off the chat list.
  A `leave_chat` for that group then gets `PERMISSION_DENIED` `You are not in this chat`:
  treat it as already left.

Details: [websocket-requests.md](../../../_docs/websocket-requests.md#remove_user).
