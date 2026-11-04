# ECHO: glossary

Plain meanings of the words used in these docs.

## The basics

| Word | What it means here |
|---|---|
| Server, backend | The program in this repository. It stores data and passes messages between people. |
| Client | The app a person uses, such as the Android app. In the WebSocket code, a "client" is one open connection, so one person on two phones is two clients. |
| REST API | Normal web requests. The app asks once and the server answers once. Used for accounts, contacts and old messages. |
| WebSocket | A connection that stays open, so the server can push things to the app at any moment. Used for live chat. |
| Request | Something an app sends over the WebSocket, such as `send_message`. |
| Event | Something the server pushes to an app, such as `message`. |
| Ack | A short "done" reply to one request. ECHO sends these with the type `response`. |

## Tools

| Word | What it means here |
|---|---|
| MongoDB | The database. It stores users, chats and messages. |
| Collection, document | A collection is like a table (`users`). A document is one record in it (one user). |
| Redis | A fast in-memory store. ECHO uses it for the logout list and to pass live messages between servers. |
| JWT, token | A signed piece of text that proves who you are. You get one at login and send it with every request. |
| Blacklist | The list of tokens cancelled by logging out. It lives in Redis. |
| bcrypt | The method used to scramble passwords before saving them. |
| Docker | A tool that runs programs in containers. We use it to start MongoDB and Redis locally, and to package the server. |
| Render | The hosting service that runs ECHO. Merging to `main` deploys automatically. |
| CI | Automatic checks that GitHub runs on every pull request. |

## Live chat

| Word | What it means here |
|---|---|
| Instance | One running copy of the server. There can be several. |
| Hub | The part of the server that tracks who is connected and which chats each connection follows. |
| Pub/sub | Short for publish and subscribe. It works like radio: one server publishes on a named channel, and every server listening hears it. |
| Fan-out | Sending one message on to many connections. |
| Following a chat | A connection receives a chat's live events. This is separate from being a member of the chat. See [WEBSOCKET_PROTOCOL.md](./WEBSOCKET_PROTOCOL.md#which-chats-you-get-live-events-for). |
| Presence | Whether someone is online. |
| Ping, pong | A small "are you there?" and "yes" between server and app. They keep a connection alive. |

## Chats

| Word | What it means here |
|---|---|
| Direct chat | A chat between two people. |
| Group | A chat with several people and roles. |
| Participant | A person listed in a chat's `participants`. This list decides who may send and read. |
| Role | `owner`, `admin` or `member`. Owners and admins can add and remove members. |
| Pending member | Someone added to a group who was never asked to accept. See #5. |
| Invite code | A code that lets someone join a group. It lasts 24 hours. |
| Cursor pagination | Loading a long list page by page. Each page tells you where the next one starts. |

"Channel" has three meanings, so check which one a doc is using:

| Phrase | Meaning |
|---|---|
| Redis channel | The named radio frequency in pub/sub, such as `chat:<chatID>`. |
| `channel` chat type | A planned broadcast chat where only admins post. It can't be created yet. |
| Go channel | A queue inside the Go code, such as `hub.Register`. |

## Project words

| Word | What it means here |
|---|---|
| Issue, `#23` | A bug or task on GitHub, by number. |
| Baseline | The test results we compare new runs against. See [current-state.md](./current-state.md#test-baseline). |
| Backfill | A one-time update that repairs existing data. |
| Race condition | A bug where two parts of a program touch the same data at the same time. |
| Increment | One small, finished piece of work. |
