# Upgrading GoBlog

Notes for upgrading an existing GoBlog site across releases that need manual steps. Newest first.

## Moving an existing site into `GOBLOG_DATA_DIR`

Optional, from 0.14.0. A site set up before `GOBLOG_DATA_DIR` existed keeps its files in goblog's working directory (`/go/src/github.com/compscidr/goblog` in the Docker image), usually with one bind mount per path. It keeps working as it is. To move it to a single data directory instead:

1. **Stop goblog** and take a backup of everything below.
2. **Copy the files into the data directory**, keeping these names:

   | Before (relative to the working directory) | In the data directory |
   |---|---|
   | `.env` | `.env` |
   | the SQLite file named by `sqlite_db` in `.env` | any name, see step 3 |
   | `www/uploads/` | `uploads/` |
   | `plugins/wasm/` | `plugins/wasm/` |
   | `plugins/dynamic/` | `plugins/dynamic/` |
   | `themes/installed/` | `themes/installed/` (not needed if you set `THEMES_INSTALLED_DIR`, which still wins) |

   Copy only what exists; goblog creates the rest. A MySQL or PostgreSQL database stays where it is.
3. **Point `sqlite_db` at the copy.** A relative path is resolved inside the data directory, so `sqlite_db=goblog.db` means `<data dir>/goblog.db`. Do not keep a `../` path: `sqlite_db=../database.db` would now resolve to the data directory's parent. An absolute path is used as given.
4. **Start goblog with `GOBLOG_DATA_DIR`** set to the directory, and with Docker mount it instead of the individual paths:

   ```bash
   docker run -p 7000:7000 -e GOBLOG_DATA_DIR=/data -v /srv/goblog:/data compscidr/goblog:v0.14.0
   ```

   The log says `Data directory: /data` at startup.
5. **Check** the site, an image from a post (uploads are served at the same `/uploads/...` URLs), **Admin → Plugins** and **Admin → Themes**. Signing in should work with the same account, since `.env` carries the session key and GitHub settings.

Two things still read only from the working directory: the WordPress-compatibility path `/wp-content/uploads/` (served from `www/`), and `local.env`, a development fallback for `.env`. Leave those mounts in place if you use them.

## 0.7.0

### Themes
The `minimal` theme is no longer built in; it is [Minimal](https://github.com/goblogplatform/goblog-theme-minimal) in the theme directory. A site whose `theme` setting is `minimal` renders `default` after the upgrade until you install Minimal from **Admin → Themes** — the setting is left alone, so the site switches back the moment the theme is installed.

### Plugins
The `analytics` and `socialicons` plugins are no longer compiled in; they are **Google Analytics** ([goblogplatform/goblog-plugin-analytics](https://github.com/goblogplatform/goblog-plugin-analytics)) and **Social Icons** ([goblogplatform/goblog-plugin-socialicons](https://github.com/goblogplatform/goblog-plugin-socialicons)) in the plugin directory. After upgrading, install the ones you use from **Admin → Plugins**. Their settings carry over (same plugin names and setting keys), so your measurement ID and profile URLs are back as soon as each plugin is installed; until then the snippet and the icon row are simply absent. Docker users: bind-mount `plugins/wasm/` first (see [Installing from the directory](README.md#installing-from-the-directory)).

## 0.3.0
The `scholar` plugin is no longer compiled in; it is now **Scholar Publications** in the plugin directory ([goblogplatform/goblog-plugin-scholar](https://github.com/goblogplatform/goblog-plugin-scholar)). After upgrading, install it from **Admin → Plugins**. Your Research page and the plugin's settings carry over (same plugin name and page type), so the page reappears in the nav as soon as the plugin is installed **and enabled** — its `enabled` setting defaults to `false` on a fresh install, while a site that already had `scholar.enabled=true` keeps it. Until then the page is hidden and `/research` answers "Page Not Available". The new plugin reads from the Semantic Scholar API only — if you were using Google Scholar, set `semantic_scholar_id` (the number at the end of your semanticscholar.org author URL) under **Admin → Settings → Scholar Publications**. Docker users: bind-mount `plugins/wasm/` first (see [Installing from the directory](README.md#installing-from-the-directory)), or the installed plugin vanishes when the container restarts.
