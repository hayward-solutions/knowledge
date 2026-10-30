# Knowledge

**Self-hosted documentation and knowledge sharing for teams that cannot use Confluence.** One `docker compose up` gives you spaces, a nested page tree, a rich block editor, version history, comments, search, and single sign-on, all running on your own infrastructure.

---

## Why Knowledge

- **A familiar shape.** Spaces with short keys, an unlimited page tree, breadcrumbs, labels, templates, and a slash-command editor, laid out the way Confluence users expect.
- **Write in a real editor.** A block editor built on ProseMirror (TipTap) with tables, task lists, syntax-highlighted code blocks, panels, expand sections, status lozenges, mentions, and internal page links.
- **Never lose work.** Drafts autosave per user, publishing records a version, and a conflict check stops two editors silently overwriting each other.
- **Find things fast.** Postgres full-text search with highlighted snippets, filters by space, label, and author, and a title quick-switcher on `/` or Cmd/Ctrl+K.
- **Plug into your existing identity.** Local email and password work out of the box; enable OIDC to get SSO from Okta, Google, Entra ID, Cognito, or anything else that speaks OpenID Connect.
- **Own your data.** Postgres for content, your own S3-compatible bucket (or the bundled RustFS) for attachments. No SaaS dependency, no seat pricing, and the browser never talks to object storage.

---

## Features at a glance

### Spaces & pages
- **Spaces** with keys such as `ENG`, a name, description, and icon, a home page, a space directory, **internal** or **private** visibility, and archiving.
- **Space roles**: admin, editor, and viewer. System admins are implicit admins in every space.
- **Page tree** with unlimited nesting, ordering, drag to move (with a Move dialog as the keyboard fallback), breadcrumbs, child page lists, and readable slugs.
- **Unpublished drafts**: a new page is visible only to its creator and space admins until it is first published.
- **Version history** with a list of versions, a text diff between any two, a side-by-side rendered comparison, and restore.
- **Trash** a page and its subtree, restore it, or purge it, with a space-level trash view. Copy a page within its space.

### Editor
- Headings, bold, italic, underline, strikethrough, highlight, text alignment, bullet and numbered lists, task lists, blockquotes, horizontal rules, links, and tables with row and column controls.
- Code blocks with syntax highlighting, and images by drop, paste, or toolbar, stored as attachments.
- Panels (info, note, success, warning, error), expand sections, status lozenges, and a table of contents.
- Internal page links and `@` mentions, each with a search picker.
- Slash command menu, bubble menu, and keyboard shortcuts.
- Autosaved drafts, a publish dialog with an optional version message, a conflict dialog when someone else published first, and a banner when someone else is editing.

### Collaboration
- **Comments** on pages with threaded replies, edit by the author, delete by the author or a space admin, and **inline comments** anchored to selected text with resolve and reopen.
- **Watch** pages and spaces and receive in-app notifications (bell, unread count, mark read) pushed over Server-Sent Events. Mentions, replies, and new comments notify the people involved.
- **Activity feed** per space, and a home dashboard of recent updates, recently viewed pages, your drafts, and starred pages.
- **Star** pages for quick return.

### Search & organisation
- **Full-text search** (Postgres, English stemming) with highlighted snippets and filters by space, label, and author, plus a title quick-switcher.
- **Labels** on pages (up to 20 each), a label index per space, and a page list per label.
- **Templates**, global and per space, with four built-in starters: meeting notes, decision record, how-to guide, and retrospective. Create a page from a template when you create the page.
- **Attachments** on pages with listing, download, delete, and image embedding. Files stream through the API into S3-compatible storage, capped at 25 MB by default.
- **Export** a page as Markdown or standalone HTML, export a whole space as a Markdown zip, and import Markdown files or folders as pages.

### Access & admin
- **Local auth** with argon2id password hashes and 30-day cookie sessions, and **OIDC SSO** with optional mapping of a group claim to system admin.
- **Admin console** for users (create, disable, reset password, grant or revoke system admin), a spaces overview, global templates, and a labels overview.
- **Per-user account settings**: display name, email, and password.
- **First-boot admin bootstrap** from environment variables.

---

## Quick start

You need Docker and Docker Compose.

```bash
git clone <this repo>
cd knowledge
cp .env.example .env
```

Edit at least `ADMIN_EMAIL` and `ADMIN_PASSWORD` in `.env`. `ADMIN_PASSWORD` must be at least 12 characters with an upper-case letter, a lower-case letter, a digit, and a symbol, or the API refuses to start. Then:

```bash
docker compose up --build
```

Open:

- Web UI: <http://localhost:3000>
- API health: <http://localhost:8080/health>
- RustFS console: <http://localhost:9001/rustfs/console/> (user and password from `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD` in `.env`)

Sign in with the admin credentials you set. From there:

1. Open **Spaces** and choose **Create space**. Pick a key such as `ENG`, a name, and a visibility.
2. In the space, choose **Create page**, write something, and **Publish**.
3. To bring in a teammate, open **Admin** in the sidebar, go to **Users**, and choose **New user**. Then open the space, go to **Settings** then **Members**, and add them with a role.

Need more depth? See the docs:

- [Configuration](docs/CONFIGURATION.md): every environment variable, storage, OIDC, roles and visibility, admin recovery.
- [API reference](docs/API.md): every endpoint with permissions, body shapes, and error codes.
- [Development](docs/DEVELOPMENT.md): running without Docker, repo layout, migrations, testing.
- [Deployment](docs/DEPLOYMENT.md): the published images, runtime constraints, and the k3s ingress layout.

---

## Roadmap

Real-time co-editing, page-level restrictions, anonymous and public spaces, PDF export beyond the print stylesheet, email delivery, Confluence XML import, macros beyond those listed, and cross-links with Backlog.

---

## License

TBD.
