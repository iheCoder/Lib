#!/bin/zsh
set -euo pipefail

# ToolHub is the only launcher installed as a user agent. Managed tools remain
# stopped until the user explicitly starts them from the dashboard.
readonly SCRIPT_DIR="${0:A:h}"
readonly REPOSITORY_ROOT="${SCRIPT_DIR:h:h}"
readonly APP_DIR="$HOME/Library/Application Support/ToolHub"
readonly BINARY_PATH="$APP_DIR/toolhub"
readonly LOG_DIR="$HOME/Library/Logs/ToolHub"
readonly PLIST_PATH="$HOME/Library/LaunchAgents/com.local.toolhub.plist"
readonly LABEL="com.local.toolhub"
readonly SERVICE_URL="http://127.0.0.1:17840"

mkdir -p "$APP_DIR" "$LOG_DIR" "${PLIST_PATH:h}"
go build -trimpath -ldflags="-s -w" -o "$BINARY_PATH" "$SCRIPT_DIR"

# plutil builds valid XML without interpolating paths into a heredoc. This
# remains safe when the repository or home directory contains spaces.
/usr/bin/plutil -create xml1 "$PLIST_PATH"
/usr/bin/plutil -insert Label -string "$LABEL" "$PLIST_PATH"
/usr/bin/plutil -insert ProgramArguments -json '[]' "$PLIST_PATH"
/usr/bin/plutil -insert ProgramArguments.0 -string "$BINARY_PATH" "$PLIST_PATH"
/usr/bin/plutil -insert ProgramArguments.1 -string "-repo" "$PLIST_PATH"
/usr/bin/plutil -insert ProgramArguments.2 -string "$REPOSITORY_ROOT" "$PLIST_PATH"
/usr/bin/plutil -insert RunAtLoad -bool true "$PLIST_PATH"
/usr/bin/plutil -insert KeepAlive -bool true "$PLIST_PATH"
/usr/bin/plutil -insert StandardOutPath -string "$LOG_DIR/service.log" "$PLIST_PATH"
/usr/bin/plutil -insert StandardErrorPath -string "$LOG_DIR/service-error.log" "$PLIST_PATH"
/usr/bin/plutil -insert ProcessType -string Background "$PLIST_PATH"

# Reinstallation replaces the existing user-scoped instance atomically enough
# that the fixed dashboard port is not left owned by the previous binary.
launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$UID" "$PLIST_PATH"
launchctl kickstart -k "gui/$UID/$LABEL"

echo "ToolHub 已安装并启动：$SERVICE_URL"
open "$SERVICE_URL"
