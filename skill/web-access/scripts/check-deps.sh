#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PORT="${CDP_PROXY_PORT:-3456}"
CHROME_PORT="${CHROME_DEBUG_PORT:-9222}"
LAUNCH_CLONE="${1:-}"

check_tcp_port() {
  local port="$1"
  node -e "
const net = require('net');
const s = net.createConnection(${port}, '127.0.0.1');
s.on('connect', () => { process.exit(0); });
s.on('error', () => process.exit(1));
setTimeout(() => process.exit(1), 2000);
" 2>/dev/null
}

if command -v node >/dev/null 2>&1; then
  NODE_VER="$(node --version 2>/dev/null)"
  NODE_MAJOR="$(echo "${NODE_VER}" | sed 's/v//' | cut -d. -f1)"
  if [ "${NODE_MAJOR}" -ge 22 ] 2>/dev/null; then
    echo "node: ok (${NODE_VER})"
  else
    echo "node: warn (${NODE_VER}, 建议升级到 22+)"
  fi
else
  echo "node: missing - 请安装 Node.js 22+"
  exit 1
fi

if check_tcp_port "${CHROME_PORT}"; then
  echo "chrome: ok (port ${CHROME_PORT})"
else
  if [ "${LAUNCH_CLONE}" = "--launch-clone" ]; then
    FALLBACK_SKILL_DIR="${WEB_ACCESS_CHROME_PROFILE_SKILL_DIR:-/Users/tal/GolandProjects/Lib/skill/chrome-profile-cdp-skill}"
    FALLBACK_SCRIPT="${FALLBACK_SKILL_DIR}/scripts/start_profiled_debug_chrome.sh"
    if [ -x "${FALLBACK_SCRIPT}" ] || [ -f "${FALLBACK_SCRIPT}" ]; then
      echo "chrome: not connected - trying isolated clone browser via chrome-profile-cdp-skill"
      PORT="${CHROME_PORT}" bash "${FALLBACK_SCRIPT}" "about:blank"
      sleep 3
    else
      echo "chrome: not connected - fallback script not found at ${FALLBACK_SCRIPT}"
      echo "请先打开 chrome://inspect/#remote-debugging 并允许当前浏览器实例远程调试"
      exit 1
    fi
  fi

  if ! check_tcp_port "${CHROME_PORT}"; then
    echo "chrome: not connected - 请打开 chrome://inspect/#remote-debugging 并勾选 Allow remote debugging"
    echo "也可运行: bash ${SKILL_DIR}/scripts/check-deps.sh --launch-clone"
    exit 1
  fi

  echo "chrome: ok (port ${CHROME_PORT})"
fi

HEALTH="$(curl -s --connect-timeout 2 "http://127.0.0.1:${PORT}/health" 2>/dev/null || true)"
if echo "${HEALTH}" | grep -q '"connected":true'; then
  echo "proxy: ready"
  exit 0
fi

if ! echo "${HEALTH}" | grep -q '"ok"'; then
  echo "proxy: starting..."
  node "${SCRIPT_DIR}/cdp-proxy.mjs" >"/tmp/cdp-proxy.log" 2>&1 &
fi

for i in $(seq 1 15); do
  sleep 1
  if curl -s "http://127.0.0.1:${PORT}/health" | grep -q '"connected":true'; then
    echo "proxy: ready"
    exit 0
  fi
  if [ "${i}" -eq 3 ]; then
    echo "chrome: 如果看到了远程调试授权弹窗，请点击允许"
  fi
done

echo "proxy: timeout - 请检查 Chrome 调试设置或查看 /tmp/cdp-proxy.log"
exit 1
