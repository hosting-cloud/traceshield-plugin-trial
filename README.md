# Traceshield plugin — 0.3.2

Connect Claude to Traceshield, then ask it to build an app or put an existing app online.
For native Claude Code on macOS and Windows x64, in the terminal and in Claude Desktop.
WSL/Linux are not validated.

## Connect

Copy this prompt into Claude and approve the browser connection when it opens.

```text
Install the Traceshield plugin traceshield-client from https://github.com/hosting-cloud/traceshield-plugin-trial at user scope using Claude's plugin installer: use claude if it's available; in Claude Desktop, use its bundled claude inside ~/Library/Application Support/Claude/claude-code on macOS or %APPDATA%\Claude\claude-code on Windows. Then run the installed plugin's bundled traceshield connect command to sign me in through the browser and remember the connection, followed by traceshield status to verify it. Read the installed skills/use-traceshield/SKILL.md with a shell command so we can use it in this conversation. Keep my other settings.
```

Then try **“Build me a notes app and put it online”** or **“Put this app online.”**

<details>
<summary>Agent installation reference</summary>

The package bundles native macOS and Windows x64 executables, a shared naturally discovered skill,
and manifests. macOS supports Intel and Apple Silicon packaging; Intel runtime has
been exercised. No separate Go, Node, GitHub CLI or global PATH installation is
needed. Git/native plugin installation preserves the executable permission.

- Terminal: use `claude plugin marketplace add https://github.com/hosting-cloud/traceshield-plugin-trial`, then `claude plugin install traceshield-client@traceshield-client-v1-trial --scope user`.
- Windows Desktop: the bundled `claude.exe` is under `%APPDATA%\Claude\claude-code\<version>\`.
- macOS Desktop Code: locate its bundled native installer with `find "$HOME/Library/Application Support/Claude/claude-code" -type f -path '*/Contents/MacOS/claude' -print -quit`. Use that quoted executable path for the same marketplace/install commands. This avoids depending on `gh` or a global CLI.
- The installed plugin directory is reported by the native plugin manager. Use its reported active version directory; older cached versions can remain after updates.
- Read `skills/use-traceshield/SKILL.md`; it selects the native Mac executable or Windows `.exe`. Displaying a compiled binary as text does not describe its commands. `--help` and authenticated `tools list` are the supported command/capability references.
- Select `bin/traceshield` on native macOS or `bin/traceshield.exe` on native Windows x64. Run that quoted installed executable with `connect`, then `status`. Connection opens browser approval and stores its credential in macOS Keychain or Windows Credential Manager. No MCP menu or global skill/agent-instruction edit is part of this package.
- Read the installed skill in the current conversation to use the CLI immediately. Fresh conversations discover that skill through the native plugin. Hosting, managed Postgres, deployment diagnostics and configuration are described by authenticated backend tools and focused guides.

Legacy snapshot upload sends only the committed app files as a standalone source snapshot
to the selected project’s private hosting repository, without local Git history.
Its receipt identifies both the local source commit and the hosted snapshot commit. Untracked local files are excluded; existing
history and origin are preserved. Account connection itself does not publish an app.
Development credentials remain private in the selected app; remote database trust
is scoped to that app with certificate and hostname verification enabled.

</details>

## Publishing in 0.3.2

`traceshield publish` sends the committed HEAD of the linked app straight to its
private Traceshield repository, deploys that exact commit and waits until it is
live. It adds no Git remote and leaves `origin`, `.git/config` and global Git
settings untouched; it never force-pushes (a diverged branch goes to
`traceshield/<sha>`). Uncommitted files stay local and are counted in the result.
`git setup` plus a manual push remains available as a fallback.

0.3.2 was exercised in native Claude Code sessions on macOS and Windows, terminal
and Desktop, Auto and Manual modes, including updates from 0.3.1. Auto mode can
still ask for confirmation during first-time setup; a user's "yes" continues it.
