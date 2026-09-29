# comments-service

GoCast comment microservice. Two-level threaded comments under streams:
top-level comments + direct replies. Bodies are markdown source sanitized
with `bluemonday` (`UGCPolicy`) — raw HTML is stripped/escaped while safe
formatting survives, and the markdown itself is untouched.

Two independent gates shape the API:

- **Write gate** — mirrors stream creation: only users with a verified email
  can post (admin bypass). Guests get `401`, unverified users `403`.
- **Read gate** — comments are exposed only on **published, non-private**
  streams (public and unlisted published streams are commentable), resolved
  via stream-service `GET /stream/:id/status`. It fails closed
  (`404`/`403`/`503`) so the read endpoints cannot be used as an
  existence/visibility oracle. Draft, processing and private streams return
  no comments, even anonymously.

Edit and delete are deliberately **not** stream-gated: authors and admins may
always curate their own comments, so a comment can still be removed after the
stream is unpublished or made private. They are also not re-checked against
email verification — the verified-email rule gates creation only.

A producer half for the activity feed: every create emits `comment.created` /
`comment.replied` (task `event:activity`, queue `events`, Redis DB 3) via its
own asynq client — best-effort, failures never fail the request. Payloads
carry `comment_id`, `parent_id` and a `snippet` (first 80 runes of the
sanitized body).

## REST API

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| GET | `/streams/:streamId/comments?limit&cursor` | public | Top-level comments, newest first. Stream-gated, rate limited. |
| GET | `/comments/:id/replies?limit&cursor` | public | Direct children, oldest first. Stream-gated (via parent), rate limited. |
| POST | `/streams/:streamId/comments` | bearer | Create (or reply via `parent_id`). Verified email required. |
| PATCH | `/comments/:id` | bearer | Edit body (author or admin). Marks `edited_at`. |
| DELETE | `/comments/:id` | bearer | Delete (author or admin). Cascade removes replies. |
| GET | `/health` | public | Probe (Postgres ping) |
| GET | `/metrics` | public | Prometheus metrics (same REST port) |

Write endpoints take `Authorization: Bearer <JWT>`; the identity RSA public
key is fetched from `JWT_ACCESS_PUBLIC_KEY_URL`. Tokens are accepted only via
the header (no `?token=` fallback).

### Pagination

Keyset (not offset) pagination, no full-table scans:

- `limit` defaults to `50`; values `> 100` are clamped to `100`, values `< 1`
  are rejected with `400 invalid limit`.
- `cursor` is base64url(`RFC3339Nano|id`) of the **last item of the previous
  page**. Malformed cursors give `400 invalid cursor`.
- Top-level comments sort `created_at DESC, id DESC` and page *before* the
  cursor; replies sort `created_at ASC, id ASC` and page *after* it.
- `next_cursor` is present only when the returned page is full
  (`len(items) == limit`), so a short/absent cursor means the end.

List response:

```json
{ "comments": [ /* ... */ ], "next_cursor": "base64url" }
```

Comment shape (`reply_count` is added on read paths to render reply badges
without a round trip):

```json
{
  "id": "uuid",
  "stream_id": "uuid",
  "user_id": "uuid",
  "user_email": "author@example.com",
  "parent_id": "uuid",
  "body": "markdown text",
  "edited_at": "RFC3339",
  "created_at": "RFC3339",
  "updated_at": "RFC3339",
  "reply_count": 3
}
```

`user_email` is denormalized from the JWT `email` claim at write time
(migration `0002_comments_user_email`, backfilled once from the shared
`users` table). Tokens without the claim leave it empty — clients fall back
to a short user id.

### Write rules

- Body is trimmed and sanitized, then **truncated to 4000 bytes**; an empty
  result gives `400 body required`.
- Replies must target a **top-level** comment on the **same stream** —
  replies to replies and cross-stream parents are both `400 invalid parent
  comment` (blocks leaking or attaching content across streams).
- A missing parent is `404 comment not found`.
- Only the author or an admin may edit/delete; anyone else gets
  `403 forbidden`.

### Errors

| Status | `error` | Cause |
| --- | --- | --- |
| 400 | `invalid id` | non-UUID path parameter |
| 400 | `invalid limit` | `limit` < 1 |
| 400 | `invalid cursor` | unparsable cursor |
| 400 | `invalid request body` | malformed JSON |
| 400 | `invalid parent_id` | non-UUID `parent_id` |
| 400 | `invalid parent comment` | reply to a reply, or parent on another stream |
| 400 | `body required` | empty after sanitization |
| 401 | `auth token required` | missing bearer on write routes |
| 401 | `invalid token` | unverifiable token |
| 401 | `invalid user id` | token without a usable user id |
| 403 | `email not verified` | create without a verified email |
| 403 | `forbidden` | not the author and not admin |
| 403 | `stream is not published` | draft/private/unpublished stream (read or write) |
| 404 | `comment not found` | unknown comment or parent |
| 404 | `stream not found` | stream-service reports 404 |
| 429 | `too many requests` | rate limit exceeded |
| 503 | `internal server error` | stream-service unreachable (fail closed) |
| 500 | `internal server error` | anything unexpected (details only in logs) |

## Rate limiting

In-memory fixed-window, per pod, no Redis round trip:

- **Reads** — `COMMENTS_READ_RATE_LIMIT` (default `300`/min) keyed on client
  IP for the public list endpoints. `TRUSTED_PROXIES` is empty by default, so
  only the direct peer (the traefik pod) is trusted and a spoofed
  `X-Forwarded-For` cannot buy a fresh budget. Consequence: all anonymous
  guests share one budget per traefik pod. An invalid `TRUSTED_PROXIES`
  value falls back to trusting nothing rather than re-enabling spoofable IPs.
- **Writes** — `WRITE_RATE_LIMIT` (default `30`/min) keyed on the JWT user
  id, so it cannot be spoofed and a botnet does not collapse into one key.

## Configuration

| Env | Default | Used by |
| --- | --- | --- |
| `SERVER_ADDR` | `:8080` | server |
| `MODE` | `debug` | gin mode (`test`/`release` also honored) |
| `CORS_ALLOW_ORIGINS` | `http://localhost:5173,https://example.com,https://comments.example.com` | CORS allowlist |
| `TRUSTED_PROXIES` | `""` (trust nothing) | client IP for read rate limiting |
| `WRITE_RATE_LIMIT` | `30` | write requests per user per minute |
| `COMMENTS_READ_RATE_LIMIT` | `300` | read requests per client per minute |
| `JWT_ACCESS_PUBLIC_KEY_URL` | — | JWT verification |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASS` / `DB_NAME` | `localhost`/`5432`/`postgres`/`""`/`postgres` | DB (deployed as `database1`) |
| `REDIS_ADDR` / `REDIS_PASS` | `localhost`/`""` | asynq producer |
| `REDIS_QUEUE_DB` | `3` | events queue DB |
| `STREAM_SERVICE_URL` | `http://stream-service:80` | stream status client (5s timeout) |
| `METRICS_ADDR` | `""` (off) | reserved; REST already serves `/metrics` |

Redis is used **only** as an asynq producer for the events queue; comments
are served synchronously from Postgres.

## Metrics

`/metrics` on the REST port exposes RED (`http_requests_total`,
`http_request_duration_seconds`) plus write counters: `comments_created_total`
(`comment`/`reply`), `comments_updated_total`, `comments_deleted_total`, and
`comments_request_duration_seconds`.

## Deployment

Schema lives in `db-migrate` (migrations `0001_comments`,
`0002_comments_user_email`) and is applied by a cluster Job:

```bash
go run ./services/db-migrate -target=comments
# or in cluster: job db-migrate-comments (`make apply-db-migrate`)
```

Manifests in `deploy/k8s/` (Deployment `comments-reader` + ClusterIP 8080).
Build context is `services/` (shared build pipeline; this service does not
import `go-shared`):

```bash
make test && make build && make push && make deploy
```

## Tests

Unit tests (no live DB) cover config, sanitize, service logic on mocks
(repo, activity recorder and stream status client via mockgen) — including
read-gate parity with the write gate and cross-stream parent rejection —
handler mapping, rate limiting, and auth middleware.
