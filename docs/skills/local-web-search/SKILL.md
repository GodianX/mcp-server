---
name: local-web-search
description: Find current library versions, API documentation, and Internet sources using pi's local-web-search MCP server when knowledge may be outdated or web research is requested.
---

# Internet research

Use the `local-web-search` MCP server configured in pi's `mcp.json`. Its tools are callable functions, not skill files or shell commands:

- `mcp__local_web_search__web_search`: find relevant pages, official documentation, releases, and changelogs.
- `mcp__local_web_search__web_fetch`: read a page returned by search or a URL supplied by the user.

1. Search when the user requests Internet research, asks about latest versions, or the answer depends on changing or uncertain facts.
2. For library questions, check the project's dependency version first. Distinguish the installed version from the latest stable release and use documentation for the relevant version.
3. Prefer official documentation, release notes, and upstream repositories. Fetch the relevant pages to verify claims beyond search snippets.
4. Follow each tool's declared argument schema. Keep queries focused and retrieve only relevant pages.
5. Answer with verified findings and supporting links. Mark unresolved uncertainty. Keep dependency upgrades within the user's requested scope.
6. If tools are unavailable or requests fail, report the limitation; present remembered information as unverified.
