# Traceshield trial plugin

Early macOS trial: browser-authorized app hosting through a bundled client.
Supports Intel and Apple Silicon packaging; Intel runtime validated.
Authentication is stored in macOS Keychain. Source upload includes committed Git history.
Requires Traceshield backend discovery endpoints. Not the active onboarding release.

## Try onboarding

Copy this single paragraph into Claude Desktop Code (This computer) or Claude Code terminal. Approve the browser connection when it opens.

```text
Install the Traceshield plugin traceshield-client from https://github.com/idan-hosting/traceshield-plugin-trial at user scope using Claude's plugin installer. In Desktop, use its bundled Claude installer under ~/Library/Application Support/Claude/claude-code; in terminal Claude, use claude. Then run the installed plugin's bundled traceshield connect command to sign me in through the browser and remember the connection, followed by traceshield status to verify it. Read the installed skills/use-traceshield/SKILL.md so we can use it in this conversation. Keep my other settings.
```
