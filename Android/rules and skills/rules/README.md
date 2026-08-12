# ECHO Android — Rules

Project-specific rules for the Android Kotlin app. Read the ones relevant to your task. These override any generic skills on conflict.

| Rule | When |
| --- | --- |
| [`clean-architecture.md`](clean-architecture.md) | Module structure, layer boundaries, feature-first |
| [`kotlin-style.md`](kotlin-style.md) | Kotlin idioms, naming, code formatting |
| [`error-handling.md`](error-handling.md) | Errors, Result, sealed classes, crash prevention |
| [`coroutines-flow.md`](coroutines-flow.md) | Coroutines, Flow, StateFlow, dispatchers |
| [`compose-ui.md`](compose-ui.md) | Jetpack Compose, state hoisting, UDF, theming |
| [`dependency-injection.md`](dependency-injection.md) | Hilt modules, scoping, injection |
| [`networking.md`](networking.md) | OkHttp, Retrofit, base URL, interceptors |
| [`websocket-client.md`](websocket-client.md) | Real-time chat client, reconnect, protocol |
| [`security.md`](security.md) | Encrypted token storage, no secrets in code |
| [`testing.md`](testing.md) | Unit tests, Compose UI tests, deterministic patterns |
| [`gradle-build.md`](gradle-build.md) | Version catalog, dependency hygiene, build config |
| [`modularity.md`](modularity.md) | Feature-first boundaries, shared core, no cross-feature imports |

Each rule has the form:

1. **What** the rule says (imperative).
2. **Why** it exists (the consequence of breaking it).
3. **How to apply** (concrete code patterns, do/don't).