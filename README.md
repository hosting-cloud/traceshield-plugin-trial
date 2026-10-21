# Traceshield plugin trial

Connect Claude to Traceshield, then ask it to build an app or put an existing app online.
For Claude Code in the macOS terminal or Desktop Code on This computer.
This is a public trial, not the active website onboarding release.

## Connect

Copy this prompt into Claude and approve the browser connection when it opens.

```text
Install the Traceshield plugin traceshield-client from https://github.com/idan-hosting/traceshield-plugin-trial at user scope using Claude's plugin installer. In Desktop, use its bundled Claude installer under ~/Library/Application Support/Claude/claude-code; in terminal Claude, use claude. Then run the installed plugin's bundled traceshield connect command to sign me in through the browser and remember the connection, followed by traceshield status to verify it. Read the installed skills/use-traceshield/SKILL.md so we can use it in this conversation. Keep my other settings.
```

Then try **“Build me a notes app and put it online”** or **“Put this app online.”**

<details>
<summary>Agent installation reference</summary>

The package bundles a compiled macOS executable, its naturally discovered skill,
and manifests. It supports Intel and Apple Silicon packaging; Intel runtime has
been exercised. No separate Go, Node, GitHub CLI or global PATH installation is
needed. Git/native plugin installation preserves the executable permission.

- Terminal: use `claude plugin marketplace add https://github.com/idan-hosting/traceshield-plugin-trial`, then `claude plugin install traceshield-client@traceshield-client-v1-trial --scope user`.
- Desktop Code: locate its bundled native installer with `find "$HOME/Library/Application Support/Claude/claude-code" -type f -path '*/Contents/MacOS/claude' -print -quit`. Use that quoted executable path for the same marketplace/install commands. This avoids depending on `gh` or a global CLI.
- The installed plugin directory is reported by the native plugin manager. Version0.1.1 is normally under `~/.claude/plugins/cache/traceshield-client-v1-trial/traceshield-client/0.1.1`.
- Read `skills/use-traceshield/SKILL.md`; its executable is `bin/traceshield`. This is a Mach-O binary, so displaying it as text does not describe its commands. `--help` and authenticated `tools list` are the supported command/capability references.
- Run the quoted installed executable with `connect`, then `status`. Connection opens browser approval and stores its credential in macOS Keychain. No MCP menu or global skill/agent-instruction edit is part of this package.
- Read the installed skill in the current conversation to use the CLI immediately. Fresh conversations discover that skill through the native plugin. Hosting, managed Postgres, deployment diagnostics and configuration are described by authenticated backend tools and focused guides.

Publishing uploads committed source and its reachable Git history to the selected
project's private hosting repository. Untracked local files are excluded; existing
history and origin are preserved. Account connection itself does not publish an app.
Development credentials remain private in the selected app; remote database trust
is scoped to that app with certificate and hostname verification enabled.

</details>
