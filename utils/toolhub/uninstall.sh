#!/bin/zsh
set -euo pipefail

readonly APP_DIR="$HOME/Library/Application Support/ToolHub"
readonly PLIST_PATH="$HOME/Library/LaunchAgents/com.local.toolhub.plist"
readonly LABEL="com.local.toolhub"

# bootout stops only ToolHub. Tools that were launched from a prior ToolHub
# process may still be running and will be rediscovered as external instances.
launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
rm -f "$PLIST_PATH"
rm -rf "$APP_DIR"

echo "ToolHub 已卸载；运行日志保留在 ~/Library/Logs/ToolHub。"
