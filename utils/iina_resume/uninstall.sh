#!/bin/zsh
set -euo pipefail

readonly APP_DIR="$HOME/Library/Application Support/IINA Resume"
readonly PLIST_PATH="$HOME/Library/LaunchAgents/com.local.iina-resume.plist"
readonly LABEL="com.local.iina-resume"

# Stop the user agent before removing its files; otherwise launchd may keep the
# already-open executable alive until the next login.
launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
rm -f "$PLIST_PATH"
rm -rf "$APP_DIR"

echo "IINA Resume 已卸载。日志仍保留在：$HOME/Library/Logs/IINA Resume"
