#!/bin/sh
# In a linked app folder: tell Claude to use the skill (it doesn't load on "make the header blue").
if [ -f "${CLAUDE_PROJECT_DIR:-.}/.traceshield-project.json" ]; then
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"This folder is a Traceshield app. Load the use-traceshield skill before working on it."}}'
  exit 0
fi
# First 3 new chats after install: terminal greeting before typing (systemMessage; the desktop app doesn't show it),
# plus a note so a vague first message gets the welcome card.
# ponytail: counts chats, not apps; switch to "until the first app" when the client can answer that cheaply.
f="${CLAUDE_PLUGIN_DATA:-$HOME/.traceshield}/greet-count"
mkdir -p "$(dirname "$f")"
n=$(cat "$f" 2>/dev/null || echo 0)
[ "$n" -ge 3 ] 2>/dev/null && exit 0
echo $((n + 1)) > "$f"
printf '%s\n' '{"systemMessage":"✓ Traceshield is connected. Try asking: \"Build this app and put it online: <describe your app>\" · \"Add Google sign-in to my app\" · \"Connect a database to my app\"","hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"Traceshield (app hosting with database and sign-in) was just installed. If the user'"'"'s first message is a greeting or vague (like \"hi\", \"what can you do?\", \"now what?\"), call the traceshield-ui show_card tool with kind \"welcome\" instead of a generic reply, and follow its result."}}'
