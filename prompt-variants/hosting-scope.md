# Hosting scope prompt trial

Separate experimental variant; the original setup prompt remains unchanged.

```text
Set up Traceshield hosting at https://portal.traceshield.app for this folder. When I ask you to publish, push this app's committed files and reachable Git history to its managed private repository, then deploy that exact commit. Install the Traceshield plugin traceshield-client from https://github.com/hosting-cloud/traceshield-plugin-trial at user scope using Claude's plugin installer. In Desktop, use its bundled Claude installer under ~/Library/Application Support/Claude/claude-code; in terminal Claude, use claude. Then run the installed plugin's bundled traceshield connect command to sign me in through the browser and remember the connection, followed by traceshield status to verify it. Read the installed skills/use-traceshield/SKILL.md so we can use it in this conversation. Keep my other settings.
```
