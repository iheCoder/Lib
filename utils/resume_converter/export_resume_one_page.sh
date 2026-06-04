#!/usr/bin/env bash
#
# 兼容旧调用方式：[输入 Markdown] [输出 PDF]。核心能力由 Node CLI 提供，
# 因而路径解析、跨平台浏览器和单页校验只有一份实现。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "用法：$0 <输入 Markdown> [输出 PDF]" >&2
  exit 2
fi

if [[ $# -eq 2 ]]; then
  exec node "$SCRIPT_DIR/bin/md-resume-pdf.js" "$1" --output "$2"
fi

exec node "$SCRIPT_DIR/bin/md-resume-pdf.js" "$1"
