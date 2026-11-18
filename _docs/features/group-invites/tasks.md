# Group invites: tasks

## Increment 1: accept, decline, pending list, backfill (REST)

- [x] 1. Invitees get a `users.chats` entry at create and add; `GetUserChats` skips pending → check: service DB tests
- [x] 2. `AcceptGroupInvite`, `DeclineGroupInvite`, `PendingGroupInvites` → check: service DB tests
- [x] 3. REST `POST groups/:groupID/accept`, `POST groups/:groupID/decline`, `GET groups/pending` → check: controller tests
- [x] 4. Startup backfill: legacy pending members (no chats entry) accepted → check: migration DB test
- [ ] 5. Docs: REST groups, decisions, Android log; local server check → check: `node scripts/verify.mjs --full`, live run
- [ ] 6. `/code-review` + `/security-review`, PR

## Increment 2: enforce pending (separate PR)

- [ ] 1. `requestStatus` checked in `chatMemberFilter` and the other membership checks; direct chats have no status
- [ ] 2. Pending members: no history, no send, no live events; protocol and Android docs

## Later (not planned yet)

- [ ] WebSocket `accept_invite` / `decline_invite` if the app needs them
- [ ] Live notice to the group and the invitee on invite, accept and decline
