# Traceshield plugin

Connect Claude Code to Traceshield, then ask it to build an app or put an existing app online.
Traceshield hosts the app with a managed Postgres database and Google sign-in.
For Claude Code on macOS and Windows x64, in the terminal and in the Claude desktop app.
WSL/Linux are not supported yet.

## Connect

Run this in Claude Code (the terminal, or the Code tab in the Claude desktop app) on macOS or Windows.
Claude chat and Cowork can't install it: if you're in one of those, open Claude Code and send the message there.

Send this as a message in a Claude Code chat and approve the browser connection when it opens.

```text
Install the Traceshield plugin traceshield-client from https://github.com/hosting-cloud/traceshield-plugin-trial at user scope using Claude's plugin installer: use claude if it's available; in Claude Desktop, use its bundled claude inside ~/Library/Application Support/Claude/claude-code on macOS or under %LOCALAPPDATA%\Claude-Data on Windows. Then run the installed plugin's bundled traceshield connect command to sign me in through the browser and remember the connection, followed by traceshield status to verify it. Read the installed skills/use-traceshield/SKILL.md with a shell command and follow its setup steps. Keep my other settings.
```

Then try **"Build me a notes app and put it online"** or **"Put this app online."**

## What this installs

| Part | What it does |
|---|---|
| `bin/traceshield` (macOS) / `bin/traceshield.exe` (Windows) | The Traceshield client, compiled from the Go source in `client/`. It signs you in through the browser, keeps that login in the macOS Keychain or Windows Credential Manager, and calls the Traceshield API to create projects and publish apps. File hashes are in `BUILD-MANIFEST.json`. The macOS build is ad-hoc signed, not notarized. |
| `skills/use-traceshield/SKILL.md` | Instructions Claude reads to use the client. |
| `hooks/session-start.sh` | Runs when a Claude Code session starts. In a folder linked to a Traceshield app, it reminds Claude to use the skill. Otherwise it shows a short "connected" greeting in the first 3 new chats after install, then does nothing. |
| `.mcp.json` → `traceshield-ui` | A remote MCP server at `portal.traceshield.app` that shows visual cards in chat (for example when an app goes live). |

The client only sends data when Claude runs a command. Publishing sends the app's committed
files and their Git history to a private repository for that app; uncommitted files stay local.
Your existing Git remotes and settings are left as they are.

## Your data

- Apps and their databases run on Hetzner servers in the EU. Each app's code is also kept in a private GitHub repository.
- Databases are backed up nightly.
- To export or delete your data, email support@traceshield.app.
- More: [traceshield.app/privacy](https://traceshield.app/privacy).

## Uninstall

1. Optional, to remove the saved login: run the client's `logout` command (ask Claude to "log out of Traceshield").
2. `claude plugin uninstall traceshield-client@traceshield-client-v1-trial`
3. `claude plugin marketplace remove traceshield-client-v1-trial`

Your apps stay online until you delete them in the portal.

<details>
<summary>Agent installation reference</summary>

The package bundles native macOS and Windows x64 executables, a shared naturally discovered skill,
and manifests. macOS builds are universal (Intel and Apple Silicon). No separate Go, Node, GitHub CLI or global PATH installation is
needed. Git/native plugin installation preserves the executable permission.

- Terminal: use `claude plugin marketplace add https://github.com/hosting-cloud/traceshield-plugin-trial`, then `claude plugin install traceshield-client@traceshield-client-v1-trial --scope user`.
- macOS Desktop Code: locate its bundled native installer with `find "$HOME/Library/Application Support/Claude/claude-code" -type f -path '*/Contents/MacOS/claude' -print -quit`. Use that quoted executable path for the same marketplace/install commands. This avoids depending on `gh` or a global CLI.
- The installed plugin directory is reported by the native plugin manager. Use its reported active version directory; older cached versions can remain after updates.
- Read `skills/use-traceshield/SKILL.md`; it selects the native Mac executable or Windows `.exe`. Displaying a compiled binary as text does not describe its commands. `--help` and authenticated `tools list` are the supported command/capability references.
- Select `bin/traceshield` on native macOS or `bin/traceshield.exe` on native Windows x64. Run that quoted installed executable with `connect`, then `status`. Connection opens browser approval and stores its credential in macOS Keychain or Windows Credential Manager. No global skill or agent-instruction edit is part of this package; the plugin's MCP server and hook are listed above.
- Read the installed skill in the current conversation to use the CLI immediately. Fresh conversations discover that skill through the native plugin. Hosting, managed Postgres, deployment diagnostics and configuration are described by authenticated backend tools and focused guides.

Legacy snapshot upload sends only the committed app files as a standalone source snapshot
to the selected project’s private hosting repository, without local Git history.
Its receipt identifies both the local source commit and the hosted snapshot commit. Untracked local files are excluded; existing
history and origin are preserved. Account connection itself does not publish an app.
Development credentials remain private in the selected app; remote database trust
is scoped to that app with certificate and hostname verification enabled.

</details>
