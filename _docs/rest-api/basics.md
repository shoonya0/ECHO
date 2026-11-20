# REST API: the basics

Base path, login header, reply shape and error codes shared by every endpoint.
Index: [REST API](./README.md).

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
(`login` refuses those accounts with the same `code`, in the usual
`success`/`message` shape, #171), and `401` with `ACCOUNT_NOT_FOUND` when the
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
| `429` | Too many attempts: `error` is `RATE_LIMITED`, and `Retry-After` gives the wait in seconds |
| `500` | Something went wrong on the server |
| `503` | The server can't check your token right now (`AUTH_UNAVAILABLE`); retry later |
