# Knowledge

**Self-hosted documentation and knowledge sharing for teams that cannot use Confluence.** One `docker compose up` gives you spaces, a nested page tree, a rich block editor, version history, comments, search, and single sign-on, all running on your own infrastructure.

---

## Why Knowledge

- **A familiar shape.** Spaces with keys, a page tree with unlimited nesting, breadcrumbs, labels, templates, and a slash-command editor, laid out the way Confluence users expect.
- **Write in a real editor.** A block editor built on ProseMirror with tables, task lists, code blocks with syntax highlighting, panels, expand sections, status lozenges, mentions, and internal page links.
- **Never lose work.** Drafts autosave per user, publishing records a version, and a conflict check stops two editors silently overwriting each other.
- **Find things fast.** Postgres full-text search with snippets, filters by space, label, and author, and a quick-switcher for titles.
- **Plug into your existing identity.** Local email and password work out of the box; enable OIDC for SSO from Okta, Google, Entra ID, Cognito, or anything else that speaks OpenID Connect.
- **Own your data.** Postgres for content, your own S3-compatible bucket (or the bundled MinIO) for attachments. No SaaS dependency and no seat pricing.

---

## Features at a glance

### Spaces and pages
- **Spaces** with keys such as `ENG`, a space home page, a space directory, private or internal visibility, and archiving.
- **Space roles**: admin, editor, and viewer. System admins are implicit admins everywhere.
- **Page tree** with unlimited nesting, ordering, drag to move, breadcrumbs, child page lists, and readable slugs.
- **Unpublished drafts** for new pages, visible only to their author and space admins.

### Editor
- Headings, lists, task lists, tables with row and column controls, code blocks, blockquotes, horizontal rules, links, and images by drop or paste.
- Internal page links with a search picker and `@` mentions.
- Panels (info, note, success, warning, error), expand sections, status lozenges, and a table of contents.
- Slash command menu, bubble menu, and keyboard shortcuts.
- Autosaved drafts, publish with an optional version message, conflict detection, and a banner when someone else is editing.

### History and collaboration
- **Version history** with a list of versions, view any version, side-by-side or inline diff, and restore.
- **Comments** on pages with threads, edit, and delete, plus inline comments anchored to text with resolve and reopen.
- **Labels** on pages and label pages across a space.
- **Watch** pages and spaces, receive in-app notifications over Server-Sent Events, and follow a per-space activity feed.
- **Star** pages and use a home dashboard of recent updates, recently viewed, your drafts, and starred pages.

### Search, files, and templates
- **Full-text search** with snippets and filters by space, label, and author.
- **Attachments** on pages with listing, download, delete, and image embedding, stored in S3-compatible storage.
- **Templates**, global and per space, with built-in starters for meeting notes, decision records, how-tos, and retrospectives.

### Trash, export, and admin
- **Trash** a page subtree, restore it, or purge it, with a space-level trash view.
- **Export** a page as Markdown or standalone HTML, export a whole space as a Markdown zip, and import Markdown files as pages.
- **Admin console** for users (create, disable, reset password, grant system admin), spaces, global templates, and global labels.
- **Local auth and OIDC SSO** with admin-group mapping, plus per-user account settings.

---

## Quick start

You need Docker and Docker Compose.

```bash
git clone <this repo>
cd knowledge
cp .env.example .env
docker compose up --build
```

Edit at least `ADMIN_EMAIL` and `ADMIN_PASSWORD` in `.env` before the first start.

Open:

- Web UI: <http://localhost:3000>
- API health: <http://localhost:8080/health>
- MinIO console: <http://localhost:9001> (credentials from `.env`)

Sign in with the admin credentials you set, create a space, and start writing pages.

More detail:

- [Configuration](docs/CONFIGURATION.md): every environment variable, storage, and OIDC.
- [API reference](docs/API.md): every endpoint.
- [Development](docs/DEVELOPMENT.md): running without Docker, repo layout, migrations, and testing.
- [Deployment](docs/DEPLOYMENT.md): running the published images.

---

## Roadmap

Real-time co-editing, page-level restrictions, anonymous and public spaces, PDF export beyond the print stylesheet, email delivery, Confluence XML import, additional macros, and cross-links with Backlog.

---

## License

TBD.
