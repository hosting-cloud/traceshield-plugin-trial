# Traceshield plugin — 0.3.1

Connect Claude to Traceshield, then ask it to build an app or put an existing app online.
For native Claude Code on macOS. This release adds an experimental Windows x64 executable;
Windows runtime validation is pending. Native Windows terminal testing comes first.
WSL/Linux and Windows Desktop are not validated. Native Windows runtime testing is still pending.

## Connect

Copy this prompt into Claude and approve the browser connection when it opens.

```text
Install the Traceshield plugin traceshield-client from https://github.com/hosting-cloud/traceshield-plugin-trial at user scope using Claude's plugin installer. In macOS Desktop, use its bundled Claude installer under ~/Library/Application Support/Claude/claude-code; in terminal Claude, use claude. Use the installed plugin's bin/traceshield on macOS or bin/traceshield.exe on native Windows x64, including Git Bash. WSL/Linux are not supported yet. If the selected binary is missing, update the plugin using Claude's plugin installer. Then run the selected binary's connect command to sign me in through the browser and remember the connection, followed by status to verify it. Read the installed skills/use-traceshield/SKILL.md so we can use it in this conversation. Keep my other settings.
```

Then try **“Build me a notes app and put it online”** or **“Put this app online.”**

<details>
<summary>Agent installation reference</summary>

The package bundles native macOS and Windows x64 executables, a shared naturally discovered skill,
and manifests. macOS supports Intel and Apple Silicon packaging; Intel runtime has
been exercised. No separate Go, Node, GitHub CLI or global PATH installation is
needed. Git/native plugin installation preserves the executable permission.

- Terminal: use `claude plugin marketplace add https://github.com/hosting-cloud/traceshield-plugin-trial`, then `claude plugin install traceshield-client@traceshield-client-v1-trial --scope user`.
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

## Git publishing in 0.2.0

The bundled Go client now configures a separate `traceshield` Git remote and
repository-scoped credential helper. Your agent commits the intended app changes,
pushes through Git, requests deployment of that exact commit and verifies the
live app. Existing origin/global Git settings stay in place. Normal Git pushes
include reachable history; untracked work stays local. Existing snapshot clients
remain supported. This update does not install a webhook or change the live
website onboarding.

The client has been exercised against the real public backend in native Mac
terminal sessions, including publishing, existing projects and saved connections.
Auto-mode results are mixed: some Git publishing attempts are blocked by the
native classifier. Manual mode uses native approvals. Final public-package and
Desktop regression checks are in progress; native Windows runtime is unverified.
