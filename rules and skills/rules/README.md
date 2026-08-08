# ECHO rules

Project-specific rules. Read the ones relevant to your task. These override any generic skills on conflict.

| Rule | When |
| --- | --- |
| [`clean-architecture.md`](clean-architecture.md) | Module structure, layer boundaries |
| [`gin-api.md`](gin-api.md) | HTTP handlers, routes, middleware |
| [`mongodb-access.md`](mongodb-access.md) | MongoDB queries, collections, transactions |
| [`logrus-logging.md`](logrus-logging.md) | Any logging |
| [`error-handling.md`](error-handling.md) | Errors, response envelope |
| [`websocket-hub.md`](websocket-hub.md) | WebSocket real-time chat, hub, pub/sub |
| [`security.md`](security.md) | Auth, JWT, rate limiting, secrets |
| [`functional-programming.md`](functional-programming.md) | Service-layer logic, pure helpers |
| [`testing.md`](testing.md) | Tests, table-driven, integration |
| [`dev-deployment.md`](dev-deployment.md) | Running locally, env config, builds |

Each rule has the form:

1. **What** the rule says (imperative).
2. **Why** it exists (the consequence of breaking it).
3. **How to apply** (concrete code patterns, do/don't).