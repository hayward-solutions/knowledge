# Configuration

Every environment variable, the storage backend, OIDC setup, the role model, and admin recovery. See [DEVELOPMENT.md](DEVELOPMENT.md) for running without Docker and [API.md](API.md) for the HTTP surface.

## Architecture

```
┌───────────────┐   ┌────────────────┐   ┌──────────────┐
│ Next.js (web) │◄─►│  Go API (chi)  │◄─►│ Postgres 16  │
│  :3000        │SSE│  :8080         │   │  :5432       │
└───────────────┘   └────────┬───────┘   └──────────────┘
                             │
                             ▼
                    ┌────────────────┐
                    │  S3 / MinIO    │
                    │  :9000/:9001   │
                    └────────────────┘
```

Attachments are proxied through the API in both directions. The browser never talks to object storage, so no public storage endpoint and no CORS configuration on the bucket are needed.

## Environment variables

Configuration is read from the `.env` file consumed by `docker-compose.yml`. `.env.example` is a copy-pasteable starting point. Variable names are identical to Backlog's.

### Database

| Var | Purpose | Default |
|---|---|---|
| `POSTGRES_DSN` | API to Postgres connection string | `postgres://knowledge:knowledge@db:5432/knowledge?sslmode=disable` |
| `POSTGRES_DB` | Database name used by the `db` service | `knowledge` |
| `POSTGRES_USER` | Database user used by the `db` service | `knowledge` |
| `POSTGRES_PASSWORD` | Database password used by the `db` service | `knowledge` |

### API and web

| Var | Purpose | Default |
|---|---|---|
| `API_PORT` | API listen port (fixed at `8080` in compose) | `8080` |
| `PUBLIC_BASE_URL` | Public URL of the web app | `http://localhost:3000` |
| `APP_ORIGINS` | Comma-separated CORS origin allowlist for the API | `http://localhost:3000` |
| `COOKIE_SECURE` | Set the `Secure` flag on the `sid` session cookie; `true` behind HTTPS | `false` |
| `NEXT_PUBLIC_API_BASE_URL` | Where the browser calls the API; a web build argument | `http://localhost:8080` |
| `VERSION` | Build argument exposed at `GET /health` and in the account menu | `dev` |
| `ADMIN_EMAIL` | First-boot admin bootstrap email | `admin@example.com` |
| `ADMIN_PASSWORD` | First-boot admin bootstrap password | `change-me` |
| `ADMIN_DISPLAY_NAME` | First-boot admin display name | `Admin` |

### Attachment storage (S3 / MinIO)

The bundled compose file runs a MinIO container and creates the bucket. Compose fixes the region, endpoint, and path style for MinIO; set them yourself when running the API outside compose or against another S3-compatible store.

| Var | Purpose | Default |
|---|---|---|
| `STORAGE_S3_BUCKET` | Bucket name used for attachments | `knowledge-dev` |
| `STORAGE_S3_REGION` | S3 region | `us-east-1` |
| `STORAGE_S3_ENDPOINT` | Endpoint the API talks to | `http://minio:9000` |
| `STORAGE_S3_FORCE_PATH_STYLE` | `true` for MinIO, RustFS, and most S3-compatible stores | `true` |
| `AWS_ACCESS_KEY_ID` | Access key | `minioadmin` |
| `AWS_SECRET_ACCESS_KEY` | Secret key | `minioadmin` |
| `ATTACHMENT_MAX_BYTES` | Maximum upload size in bytes | `26214400` (25 MB) |
| `MINIO_ROOT_USER` | Root user for the MinIO container | `minioadmin` |
| `MINIO_ROOT_PASSWORD` | Root password for the MinIO container | `minioadmin` |

### OIDC single sign-on (optional)

Leave all `OIDC_*` unset to disable SSO and keep password-only login.

| Var | Purpose | Default |
|---|---|---|
| `OIDC_ISSUER_URL` | Issuer URL (required to enable SSO) | unset |
| `OIDC_CLIENT_ID` | OAuth client id | unset |
| `OIDC_CLIENT_SECRET` | OAuth client secret | unset |
| `OIDC_REDIRECT_URL` | Callback URL registered with the IdP, e.g. `http://localhost:8080/api/v1/auth/oidc/callback` | unset |
| `OIDC_SCOPES` | Comma- or space-separated scopes | unset |
| `OIDC_ADMIN_GROUP` | Group claim value that grants system admin; blank disables auto-admin | unset |
| `OIDC_GROUPS_CLAIM` | Claim carrying group membership | `groups` |
| `OIDC_EMAIL_CLAIM` | Claim carrying the user's email | `email` |
| `OIDC_NAME_CLAIM` | Claim carrying the display name | `name` |
| `OIDC_PROVIDER_NAME` | Label on the "Sign in with" button | `SSO` |

## Roles and permissions

Space roles from least to most privileged: viewer, editor, admin. System admins implicitly have admin access to every space.

| Action | system admin | space admin | editor | viewer |
|---|:-:|:-:|:-:|:-:|
| View pages, history, attachments | yes | yes | yes | yes |
| Comment (own comments), star, watch | yes | yes | yes | yes |
| Create, edit, publish, move, trash, restore pages | yes | yes | yes | no |
| Upload attachments, edit labels, create templates | yes | yes | yes | no |
| Space settings, members, archive, delete, purge trash | yes | yes | no | no |
| Manage users, global templates and labels | yes | no | no | no |

Internal spaces make every signed-in user an implicit viewer; private spaces are members only. Pages that were never published are visible only to their creator and space admins. The exact matrix lives in `api/internal/domain/permissions.go`.

## First-boot admin bootstrap

On startup the API runs migrations, then creates a system admin from `ADMIN_EMAIL`, `ADMIN_PASSWORD`, and `ADMIN_DISPLAY_NAME` if the `users` table is empty. If users already exist the variables are ignored.

## Resetting a lost admin password

Sign in as another system admin and reset the password from the admin console. If no admin can sign in, wipe the users table and let the bootstrap run again:

```bash
docker compose exec db psql -U knowledge -d knowledge -c "DELETE FROM users;"
docker compose restart api
```

This also deletes every other user, membership, and session.
