#!/bin/zsh
set -euo pipefail

# This installer deliberately stays user-scoped: the helper only needs access
# to this user's IINA preferences and should never require administrator rights.
readonly SCRIPT_DIR="${0:A:h}"
readonly APP_DIR="$HOME/Library/Application Support/IINA Resume"
readonly BINARY_PATH="$APP_DIR/iina-resume"
readonly LOG_DIR="$HOME/Library/Logs/IINA Resume"
readonly PLIST_PATH="$HOME/Library/LaunchAgents/com.local.iina-resume.plist"
readonly LABEL="com.local.iina-resume"
readonly SERVICE_URL="http://127.0.0.1:17845"

mkdir -p "$APP_DIR" "$LOG_DIR" "${PLIST_PATH:h}"

# Build into the final application directory so launchd never depends on the
# repository remaining at its current path.
go build -trimpath -ldflags="-s -w" -o "$BINARY_PATH" "$SCRIPT_DIR"

cat > "$PLIST_PATH" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array><string>$BINARY_PATH</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$LOG_DIR/service.log</string>
  <key>StandardErrorPath</key><string>$LOG_DIR/service-error.log</string>
  <key>ProcessType</key><string>Background</string>
</dict>
</plist>
PLIST

# bootout is best-effort because the label is absent on the first install.
launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$UID" "$PLIST_PATH"
launchctl kickstart -k "gui/$UID/$LABEL"

echo "IINA Resume 已安装并启动：$SERVICE_URL"
echo "建议把这个地址加入浏览器书签或固定标签页。"
open "$SERVICE_URL"
