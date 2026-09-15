# comments-service

GoCast comment microservice. Two-level threaded comments under streams:
top-level comments + direct replies (replies to replies are rejected).
Comment bodies are markdown-formatting strings sanitized with
`bluemonday` (UGC allowlist). Write gating mirrors stream creation: only
users with a verified email can post (admin bypass). Read API is public.

A producer half for the activity feed: every create emits `comment.created`
/ `comment.replied` (task `event:activity`, queue `events`, Redis DB 3) via
its own asynq client — best-effort, failures never fail the request.

## REST API

| Method | Path | Description |
| --- | --- | --- |
| GET | `/streams/:streamId/comments?limit&cursor` | Top-level comments, newest first. Public. |
| GET | `/comments/:id/replies?limit&cursor` | Direct children, oldest first. Public. |
| POST | `/streams/:streamId/comments` | Create (or reply via `parent_id`). Verified email required. |
| PATCH | `/comments/:id` | Edit body (author or admin). Marks `edited_at`. |
| DELETE | `/comments/:id` | Delete (author or admin). Cascade removes replies. |
| GET | `/health` | Probe (DB check) |
| GET | `/metrics` | Prometheus metrics |

List response: `{"comments": [...], "next_cursor": "..."}`. `next_cursor` is
base64(`RFC3339Nano|id`) of the last item and appears when the page is full.
Comment shape:

```json
{
  "id": "uuid",
  "stream_id": "uuid",
  "user_id": "uuid",
  "parent_id": "uuid",
  "body": "markdown text",
  "edited_at": "RFC3339",
  "created_at": "RFC3339",
  "reply_count": 3
}
```

Write endpoints require `Authorization: Bearer <JWT>` (identity RSA public
key from `JWT_ACCESS_PUBLIC_KEY_URL`). Errors map to:
`403 email not verified`, `403 forbidden`, `404 comment not found`,
`400 invalid parent comment`, `400 body required`.

## Configuration

| Env | Default | Used by |
| --- | --- | --- |
| `SERVER_ADDR` | `:8080` | server |
| `MODE` | `debug` | gin mode |
| `CORS_ALLOW_ORIGINS` | `"http://localhost:5173,https://example.com,https://comments.example.com"` | server CORS |
| `JWT_ACCESS_PUBLIC_KEY_URL` | — | JWT verification |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASS` / `DB_NAME` | `localhost`/`5432`/`postgres`/`""`/`postgres` | DB |
| `REDIS_ADDR` / `REDIS_PASS` | `localhost`/`""` | asynq producer |
| `REDIS_QUEUE_DB` | `3` | events queue DB |
| `METRICS_ADDR` | `""` (off) | (reserved; REST serves `/metrics`) |

## Deployment

Schema via `db-migrate`:

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
(repo + recorder via mockgen), handler mapping, and auth middleware.