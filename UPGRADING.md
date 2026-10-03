# Upgrading GoBlog

Notes for upgrading an existing GoBlog site across releases that need manual steps. Newest first.

## 0.7.0

### Themes
The `minimal` theme is no longer built in; it is [Minimal](https://github.com/goblogplatform/goblog-theme-minimal) in the theme directory. A site whose `theme` setting is `minimal` renders `default` after the upgrade until you install Minimal from **Admin → Themes** — the setting is left alone, so the site switches back the moment the theme is installed.

### Plugins
The `analytics` and `socialicons` plugins are no longer compiled in; they are **Google Analytics** ([goblogplatform/goblog-plugin-analytics](https://github.com/goblogplatform/goblog-plugin-analytics)) and **Social Icons** ([goblogplatform/goblog-plugin-socialicons](https://github.com/goblogplatform/goblog-plugin-socialicons)) in the plugin directory. After upgrading, install the ones you use from **Admin → Plugins**. Their settings carry over (same plugin names and setting keys), so your measurement ID and profile URLs are back as soon as each plugin is installed; until then the snippet and the icon row are simply absent. Docker users: bind-mount `plugins/wasm/` first (see [Installing from the directory](README.md#installing-from-the-directory)).

## 0.3.0
The `scholar` plugin is no longer compiled in; it is now **Scholar Publications** in the plugin directory ([goblogplatform/goblog-plugin-scholar](https://github.com/goblogplatform/goblog-plugin-scholar)). After upgrading, install it from **Admin → Plugins**. Your Research page and the plugin's settings carry over (same plugin name and page type), so the page reappears in the nav as soon as the plugin is installed **and enabled** — its `enabled` setting defaults to `false` on a fresh install, while a site that already had `scholar.enabled=true` keeps it. Until then the page is hidden and `/research` answers "Page Not Available". The new plugin reads from the Semantic Scholar API only — if you were using Google Scholar, set `semantic_scholar_id` (the number at the end of your semanticscholar.org author URL) under **Admin → Settings → Scholar Publications**. Docker users: bind-mount `plugins/wasm/` first (see [Installing from the directory](README.md#installing-from-the-directory)), or the installed plugin vanishes when the container restarts.
