# GoBlog
[![Build and Test](https://github.com/goblogplatform/goblog/actions/workflows/push.yml/badge.svg)](https://github.com/goblogplatform/goblog/actions/workflows/push.yml)
[![codecov](https://codecov.io/gh/goblogplatform/goblog/branch/main/graph/badge.svg)](https://codecov.io/gh/goblogplatform/goblog)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

GoBlog is a simple, self-hosted blogging platform written in Go, with Markdown posts, themes, plugins, comments and an admin dashboard. Official site: https://www.goblog.live

Upgrading an existing site? See [UPGRADING.md](UPGRADING.md) for release-specific steps.

## Features

### Content
- Markdown posts with code syntax highlighting and table support
- Draft / publish workflow
- Post revision history with rollback
- Tags with tag cloud
- Configurable post types (blog posts, notes, etc.)
- Full-text search
- File uploads (images, PDFs, etc.)
- Internal and external backlink tracking
- Comments with markdown support, spam honeypot, and rate limiting; by default commenters must be logged in (GitHub or email code)
- RSS-ready sitemap generation

### Pages
- Configurable dynamic pages (writing, archives, tags, about, custom), plus pages owned by plugins
- Research page listing your publications from Semantic Scholar — install **Scholar Publications** from Admin → Plugins
- Archives sorted by year and month

### Theming
- WordPress-style theme system (`themes/{name}/`)
- Switch themes from admin settings without restart (hot-reload)
- One built-in theme, `default` (monospace, gray); [Minimal](https://github.com/goblogplatform/goblog-theme-minimal) and [Forest](https://github.com/goblogplatform/goblog-theme-forest) install from the theme directory
- Theme-specific CSS served at `/theme/`
- Custom header/footer code injection via settings (for analytics, etc.)

### Admin
- GitHub OAuth login
- Install wizard for first-time setup
- Admin dashboard with recent comments, and a paginated comments page for moderation
- Configurable settings (site title, subtitle, social URLs, favicon, etc.)
- Post type management
- Page management with hero images/videos

### Plugins
- Plugin system for injecting template data / HTML, scheduled jobs, settings, and whole pages
- Built-in plugins: `directory` (the plugin and theme directory that runs [goblog.live/plugins](https://goblog.live/plugins) and [goblog.live/themes](https://goblog.live/themes); off by default) and `docs` (the builder docs at [goblog.live/docs](https://goblog.live/docs); off by default). Google Analytics, Social Icons and Scholar Publications are directory plugins.
- Dynamic plugins: drop a `.go` file in `plugins/dynamic/` — no rebuild (see [Plugins](#plugins))
- WebAssembly plugins (sandboxed, any language/dependencies) installable from the directory
- Install plugins from the [directory](https://www.goblog.live/plugins) with one click under **Admin → Plugins**
- Built-in documentation plugin (`docs`): the plugin/theme builder docs served at `/docs`; what goblog.live/docs runs

### Infrastructure
- SQLite (file-based, zero config), MySQL, or PostgreSQL
- Docker support with tagged releases on Docker Hub
- Configurable trusted proxies for reverse proxy deployments (`TRUSTED_PROXIES` env var)
- GitHub Actions CI/CD

## Quick Start

### Local
```bash
go build
./goblog
```
Visit http://localhost:7000 and follow the install wizard.

The wizard first asks for a **setup code**, which goblog prints to its log at startup (`GoBlog setup code: XXXX-XXXX-XXXX`; with Docker, `docker logs <container> 2>&1 | grep "setup code"`). Being able to read the server's log is the proof that you run the server, so nobody else who finds a half-installed site can finish the install for you. The code changes on every restart and stops being printed once the site has an admin.

It then takes three steps: the database, the site's title and images, and an **admin account** with an email and a password. No GitHub OAuth app or mail server is needed; the wizard still offers GitHub as the alternative for the last step.

### Docker
```bash
docker run -p 7000:7000 -e GOBLOG_DATA_DIR=/data -v goblog-data:/data compscidr/goblog:latest
```
`GOBLOG_DATA_DIR` is where goblog keeps everything it writes: `.env` (the session key, database settings and GitHub credentials the wizard saves), the SQLite database, uploads, and installed plugins and themes. With it on a volume, the site survives the container being replaced, for example when you pull a newer image. Without it those files are written inside the container and are lost with it.

A relative SQLite path such as the wizard's default `goblog.db` is created inside the data directory; an absolute path is used as given. Sites set up before `GOBLOG_DATA_DIR` existed keep working unchanged when it is not set.

### Database
SQLite is the default and needs no setup. To use MySQL or PostgreSQL instead, pick it in the install wizard or set the variables in `.env` (see `template.env`):
```bash
database=postgres
POSTGRES_HOST=localhost
POSTGRES_PORT=5432          # default
POSTGRES_USER=goblog
POSTGRES_PASSWORD=...
POSTGRES_DATABASE=goblog
POSTGRES_SSLMODE=disable    # default; or require / verify-ca / verify-full
```
The schema is created and migrated automatically on startup for all three. There is no built-in tool for moving an existing site between databases.

### Behind a Reverse Proxy
Set `TRUSTED_PROXIES` so `X-Forwarded-For` headers are trusted for client IP resolution:
```bash
TRUSTED_PROXIES=172.16.0.0/12 ./goblog
```

The session cookie is `HttpOnly`, `SameSite=Lax` and `Secure`, so it is only sent over HTTPS (browsers exempt `localhost`, so local development on `http://localhost:7000` still works). If you serve goblog over plain HTTP on any other host, set `SESSION_SECURE=false` or logins will not stick. Mutating `/api/v1` requests must be sent as `application/json` (`/api/v1/upload` as `multipart/form-data`); anything else gets `415 Unsupported Media Type`.

### Admin Password
The admin account the wizard creates signs in on the login page with its email and password. The email is only a sign-in name; goblog sends nothing to it. Passwords are at least 10 characters and stored as bcrypt hashes, and sign-in attempts are rate limited per client address.

If you forget the password, reset it from the server. The command prints a new random password and signs out any browser logged in as that account:
```bash
./goblog reset-admin-password                # or: docker exec <container> ./goblog reset-admin-password
```
Run it from goblog's working directory, or with `GOBLOG_DATA_DIR` set as it is for the server, so that it finds `.env`.

There is one password account, the one the wizard creates. To add GitHub login to such a site later, put `client_id` and `client_secret` in `.env` and restart; the login page then offers both.

### Pinning the Admin Account
If you chose GitHub in the wizard, or pre-populate `.env` with GitHub credentials and skip it, the first GitHub account to complete login becomes the admin. If you pre-populate `.env` (e.g. from configuration management) and skip the wizard, anyone could win that race. Pin it to your own account by adding either or both of these to `.env`:
```bash
admin_login=your-github-username      # case-insensitive
admin_github_id=12345                 # numeric id: https://api.github.com/users/your-github-username
```
Other accounts can still log in as regular users but are never promoted. Leave both unset to keep the first-to-login behaviour.

### Managing Admins
The pin above only decides who becomes the *first* admin. After that, admins are managed from the **Users** page in the admin area (`/admin/users`), which lists everyone who has logged in. An existing admin can promote any GitHub user to admin or demote another admin (including the wizard's password account); the last remaining admin can't be demoted, so the site never ends up with none. Email-login users can't be made admin (see #565).

To hand the site over to a different GitHub account: log in with the new account once so it appears in the list, promote it from your current admin account, then log in as the new account and demote the old one.

### Email Login (one-time codes)
Visitors without a GitHub account can log in with an emailed 6-digit code. Add SMTP details to `.env`:
```bash
smtp_host=smtp.example.com
smtp_port=587                         # 465 for implicit TLS; anything else uses STARTTLS when offered
smtp_user=postmaster@example.com      # omit for an unauthenticated relay
smtp_password=...
smtp_from=blog@example.com
```
When `smtp_host` and `smtp_from` are both set the login page offers "sign in with email"; otherwise it shows GitHub only. Codes expire after 10 minutes, allow 5 wrong attempts, and can be re-requested once a minute. Email users are regular users — they cannot be made admin (see above).

SMTP settings are read once at startup, so restart goblog after changing any `smtp_*` value in `.env` for the change to take effect. Go's SMTP client only sends `smtp_user`/`smtp_password` over an encrypted connection (STARTTLS, or implicit TLS on port 465) unless the host is `localhost`, so if you need an unencrypted remote relay, use it without credentials.

### Comments and Login
Comments require a logged-in user by default: the comment form is replaced by a "Log in to leave a comment" link, and a comment is attributed to the account that posted it (the email is always the account's; the name defaults to the GitHub name or the email's local part but can be edited per comment). Since email login is the way most readers will get an account, configure SMTP as above. If you would rather allow anonymous comments — for example on a site with GitHub login only — untick **comments_require_login** on the admin settings page.

## Theming

Themes live in `themes/{name}/` with this structure:
```
themes/
  default/
    templates/    # HTML templates
    static/       # CSS and assets (served at /theme/)
  installed/      # themes installed from the directory (THEMES_INSTALLED_DIR; bind-mount it in Docker)
    ocean/
      templates/
      static/
```

A theme's templates are loaded **on top of `themes/default`**: it only has to ship the templates it changes, and everything else — including admin pages added by newer goblog releases — renders from default. `/theme/<file>` serves the active theme's `static/` and falls back to default's.

To create a custom theme:
1. Create `themes/my-theme/templates/` and copy in only the templates you want to change (start with `header.html`, `footer.html`, `home.html`); add `static/` for CSS.
2. Set the `theme` setting to `my-theme` in admin settings (hot-reloads, no restart).

Full guide: [goblog.live/docs/writing-a-theme](https://www.goblog.live/docs/writing-a-theme).

**Admin → Themes** browses the [theme directory](https://www.goblog.live/themes), installs a theme into `themes/installed/` (bind-mount it in Docker, set with `THEMES_INSTALLED_DIR`, or installs vanish on restart), activates it, updates it when the directory has a newer release, and removes it. The directory URL is the `theme_directory_url` setting; see [Publishing a theme](https://www.goblog.live/docs/publishing-a-theme) to publish one.

A theme is code: once activated its templates render every page, including the admin, with the same template functions and data goblog's own templates get. The directory's validation checks that a theme is well-formed, not that it is benign, and a listing on goblog.live is a maintainer's approval, not a code audit — install only themes you trust, as with plugins.

## Plugins

A plugin implements the `plugin.Plugin` interface (`plugin/plugin.go`). Embed `plugin.BasePlugin` to get no-op defaults and implement only the hooks you need:

| Hook | What it does |
|---|---|
| `Name()`, `DisplayName()`, `Version()` | Identity. `Name()` is the unique key used to store the plugin's settings. |
| `Settings()` | Declares settings. They appear on the plugin's own page under **Admin → Plugins**, are stored in `plugin_settings`, and reach every hook as strings via `ctx.Settings`. The admin UI renders `Type: "textarea"` as a textarea and everything else as a single-line text input (there is no file or checkbox widget for plugin settings yet, so store booleans as `"true"`/`"false"`). Declare an `enabled` setting to get the on/off toggle — the registry calls every plugin regardless, so honour `ctx.Settings["enabled"]` yourself. |
| `TemplateHead(ctx)` / `TemplateFooter(ctx)` | Return raw HTML injected into `<head>` / before `</body>` on every rendered page. Escape anything that came from settings or the request. |
| `TemplateData(ctx)` | Returns data made available to templates as `.plugins.<name>`. |
| `ScheduledJobs()` | Periodic background jobs (`Name`, `Interval`, `Run(db, settings)`), started at boot. |
| `Pages()` / `RenderPage(ctx, pageType)` | Own a page type: it gets a slug, an optional nav entry, and you choose the template and data when it is visited. The plugin also owns everything under its slug: `ctx.SubPath` is `""` for `/research`, `"2024"` for `/research/2024`. Return a template name to render it inside the theme, or write the response yourself (e.g. `ctx.GinContext.JSON(...)`) and return `""`; returning `""` without writing anything gives a 404. `plugins/directory` is the example: it serves `/plugins`, `/plugins/<name>` and `/plugins/index.json`. |
| `OnInit(db)` | Runs once at startup, after settings are seeded. |

`ctx` is a `*plugin.HookContext` carrying the Gin context, the DB, the plugin's own settings, the template being rendered, and the existing template data. [goblog-plugin-hello](https://github.com/goblogplatform/goblog-plugin-hello) is the smallest complete example (as a WebAssembly plugin; the exports map one-to-one onto these hooks).

### Compiled-in plugins
Live in `plugins/<name>/` as a normal Go package, and are registered in `main()`:
```go
registry.Register(myplugin.New())
```
They have full access to `gin`, `gorm`, and any module dependency, and are part of the release binary. Use this for anything that ships with goblog.

### WebAssembly plugins
The plugin directory installs sandboxed WebAssembly modules built with [Extism](https://extism.org/): any language with an Extism PDK, any dependencies, no goblog rebuild. A plugin has no filesystem access and can reach the network only at the hosts it declares.

Every export takes and returns JSON through Extism's input/output. Only `identity` is mandatory; a missing export behaves like `BasePlugin`'s no-op.

The full contract — every export's input and output, `ctx`, host functions, store limits, timeouts — is documented at [goblog.live/docs/plugin-api](https://www.goblog.live/docs/plugin-api) (or `/docs/plugin-api` on any goblog with the `docs` plugin enabled); the source of those pages is `plugins/docs/content/`.

Build one with the standard Go toolchain and [`github.com/extism/go-pdk`](https://github.com/extism/go-pdk):
```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
```
[`plugin/wasm/testdata/echo/main.go`](plugin/wasm/testdata/echo/main.go) is the reference implementation of every export, including the store and an outbound HTTP call.

An installed wasm plugin is `plugins/wasm/<name>.wasm` plus a `<name>.json` sidecar declaring its allowed hosts:
```json
{"allowed_hosts": ["api.example.test"]}
```
The installer writes this sidecar for directory installs; an operator dropping a `.wasm` file by hand can ship its own (no sidecar means no network access). Loading is on by default — set `ENABLE_WASM_PLUGINS=false` to turn it off.

`goblog validate-plugin <file.go|file.wasm>` with a `.wasm` file loads the module with no store or network access, calls `identity`/`settings`/`pages`/`jobs`, and prints the identity as JSON:
```bash
./goblog validate-plugin plugin.wasm
# {"name":"echo","display_name":"Echo","version":"1.2.3","runtime":"wasm"}
```

#### Installing from the directory
**Admin → Plugins** lists what is installed and lets you browse and search the [plugin directory](https://www.goblog.live/plugins), install a plugin with one click, update it when the directory has a newer release, or uninstall it. WebAssembly is the only format the directory installs; requirements:
- `plugins/wasm/` writable by goblog (WASM loading is on by default; set `ENABLE_WASM_PLUGINS=false` to disable it entirely). With Docker, set `GOBLOG_DATA_DIR` (see [Docker](#docker)) or bind-mount that directory (see below) — otherwise installed plugins vanish with the container.
- The directory URL is the `plugin_directory_url` setting (default `https://www.goblog.live/plugins/index.json`); point it elsewhere to run a private directory. `plugin_directory_url` is a trust decision: whatever it points at can offer code that runs inside goblog once you click Install.

Install downloads the plugin's `.wasm` asset, verifies its sha256 against the directory index, loads it, checks that its name and version match, and only then writes it (plus the `allowed_hosts` sidecar) to `plugins/wasm/` and starts it — no restart. Updates keep the plugin's settings; uninstall removes both files. Install only from sources you trust. A dynamic (`.go`) plugin installed before the directory went wasm-only can still be updated to a wasm release or uninstalled, just not reinstalled as `.go`.

### Dynamic plugins
Yaegi plugins are the local/operator path for extending goblog without a rebuild — not a directory-installable format (see WebAssembly plugins above for that). Loaded at startup (or, for one previously installed from the directory, kept running) from `plugins/dynamic/*.go` by the embedded [Yaegi](https://github.com/traefik/yaegi) Go interpreter. Enable with:
```bash
ENABLE_DYNAMIC_PLUGINS=true ./goblog
```
A dynamic plugin is a single `package main` file defining `func NewPlugin() plugin.Plugin`. Start from the shipped example:
```bash
cp plugins/dynamic/hello.go.example plugins/dynamic/hello.go
ENABLE_DYNAMIC_PLUGINS=true ./goblog     # every page now ends with a greeting
```
then edit the message under **Admin → Settings → Hello (example)**.

Limits of the interpreted environment:
- Available imports are the Go standard library and `goblog/plugin` (`Plugin`, `BasePlugin`, `HookContext`, `SettingDefinition`, `ScheduledJob`, `PageDefinition`). `gin` and `gorm` are **not** available, so the hooks that name their types — `TemplateData`, `ScheduledJobs`, `OnInit`, `RenderPage` — can't be implemented dynamically; write a compiled-in plugin for those.
- A file that fails to load is logged and skipped; the rest still load.
- Dynamic plugins run as ordinary Go code inside the goblog process with stdlib access, unsandboxed. Only load files you control; `plugins/dynamic/` should be writable by the operator alone.

With Docker, bind-mount the directory and set the flag:
```bash
docker run -p 7000:7000 -e ENABLE_DYNAMIC_PLUGINS=true \
  -v $PWD/plugins/dynamic:/go/src/github.com/compscidr/goblog/plugins/dynamic \
  -v $PWD/plugins/wasm:/go/src/github.com/compscidr/goblog/plugins/wasm \
  -v $PWD/themes/installed:/go/src/github.com/compscidr/goblog/themes/installed \
  compscidr/goblog:latest
```

#### Checking a plugin file
`goblog validate-plugin <file>` accepts either a Yaegi `.go` file or a wasm `.wasm` module, loads it in isolation, and prints its identity as JSON (exit 1 with the load error on stderr if it fails):
```bash
./goblog validate-plugin plugins/dynamic/hello.go.example
# {"name":"hello","display_name":"Hello (example)","version":"1.0.0"}
./goblog validate-plugin plugin.wasm
# {"name":"echo","display_name":"Echo","version":"1.2.3","runtime":"wasm"}
```
With the Docker image (its entrypoint is a shell command, so override it):
```bash
docker run --rm --network none -v "$PWD:/p" --entrypoint /go/src/github.com/compscidr/goblog/goblog \
  compscidr/goblog:latest validate-plugin /p/plugin.wasm
```
This is the same check goblog.live runs on every submission to the plugin directory.

### Plugin directory
[goblog.live/plugins](https://goblog.live/plugins) lists published plugins; `https://goblog.live/plugins/index.json` is the same list as JSON (name, version, author, license, `download_url`, `sha256`, `min_goblog_version`, `runtime`, `allowed_hosts`) and `/plugins/<name>.json` carries one plugin's README, changelog and release history — full field-by-field detail at [Directory formats](https://www.goblog.live/docs/directory-formats). Plugins are individual GitHub repositories with releases — see [Publishing a plugin](https://www.goblog.live/docs/publishing-a-plugin). To publish one, paste its URL at [goblog.live/plugins/submit](https://goblog.live/plugins/submit): it is validated on the spot (latest release, manifest, `plugin.wasm` loads and its name/version match) and listed once a maintainer approves it.

The directory is the built-in `directory` plugin, so any goblog can host one: turn it on under **Admin → Settings → Plugin Directory** (`enabled` = `true`). Submissions are stored in the site's database and reviewed under **Admin → Plugins → Directory**, where you can also add repositories yourself, rebuild an entry or delist it. Listed plugins are re-checked every `refresh_minutes` (default 360) for new releases and star counts. The GitHub API allows 60 anonymous requests per hour; set `github_token` (any token, no scopes needed) to raise that to 5000 if you list more than a handful of plugins. README, changelog and release-note HTML is rendered by GitHub's markdown API and shown as-is on the directory pages; the admin sees it in the pending card before approving.

## Sites running GoBlog

- [goblog.live](https://www.goblog.live) — the official GoBlog site, with docs and the plugin and theme directories
- [jasonernst.com](https://www.jasonernst.com)

Running GoBlog yourself? [Open a pull request](https://github.com/goblogplatform/goblog/edit/main/README.md) adding your site to this list.

## Testing
```bash
go test ./...
```

The install smoke test builds the Docker image and walks the install wizard against a fresh database (needs Docker and curl):
```bash
scripts/install-smoke-test.sh sqlite   # or mysql, postgres
```

## Architecture

- **Gin** for HTTP routing and middleware
- **GORM** for database ORM (SQLite, MySQL support)
- **Showdown.js** + **DOMPurify** for client-side markdown rendering
- **Bootstrap 5** for UI framework
- Server-side rendered templates with JSON REST API at `/api/v1/`
