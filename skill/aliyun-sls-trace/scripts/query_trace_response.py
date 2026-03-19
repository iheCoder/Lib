#!/usr/bin/env python3
"""Query SLS by custom query or trace id and print matched payloads."""

import argparse
import json
import os
import subprocess
import tempfile
from datetime import datetime
from pathlib import Path

SDK_VERSION = "v0.1.117"
TEMP_GO_MOD = f"""module aliyun-sls-trace-tmp

go 1.20

require github.com/aliyun/aliyun-log-go-sdk {SDK_VERSION}
"""


GO_TEMPLATE = r'''
package main

import (
  "encoding/json"
  "fmt"
  sls "github.com/aliyun/aliyun-log-go-sdk"
  "os"
  "strings"
)

func main() {
  endpoint := os.Getenv("SLS_ENDPOINT")
  ak := os.Getenv("SLS_ACCESS_KEY_ID")
  sk := os.Getenv("SLS_ACCESS_KEY_SECRET")
  project := os.Getenv("SLS_PROJECT")
  logstore := os.Getenv("SLS_LOGSTORE")
  traceID := os.Getenv("TRACE_ID")
  queryText := os.Getenv("QUERY_TEXT")
  fromSec := int64(__FROM_SEC__)
  toSec := int64(__TO_SEC__)
  lines := int64(__LINES__)
  offset := int64(__OFFSET__)

  client := &sls.Client{Endpoint: endpoint, AccessKeyID: ak, AccessKeySecret: sk}
  queries := make([]string, 0, 2)
  if strings.TrimSpace(queryText) != "" {
    queries = append(queries, queryText)
  } else {
    queries = append(queries, "traceID: "+traceID, "\""+traceID+"\"")
  }

  for _, q := range queries {
    req := &sls.GetLogRequest{
      From: fromSec,
      To: toSec,
      Lines: lines,
      Offset: offset,
      Reverse: true,
      Query: q,
    }
    resp, err := client.GetLogsV2(project, logstore, req)
    if err != nil {
      fmt.Printf("QUERY_ERROR query=%s err=%v\n", q, err)
      continue
    }
    fmt.Printf("QUERY=%s MATCHED=%d\n", q, len(resp.Logs))
    for i, m := range resp.Logs {
      logsStr := m["logs"]
      if logsStr == "" {
        fmt.Printf("ITEM=%d RAW=%v\n", i, m)
        continue
      }
      var entries []map[string]any
      if err := json.Unmarshal([]byte(logsStr), &entries); err != nil {
        continue
      }
      for _, e := range entries {
        name, _ := e["name"].(string)
        attr, _ := e["attribute"].(map[string]any)
        if attr == nil {
          continue
        }
        for _, key := range []string{"response", "result", "body", "output"} {
          if v, ok := attr[key]; ok {
            s := strings.TrimSpace(fmt.Sprintf("%v", v))
            if s == "" {
              continue
            }
            fmt.Printf("ITEM=%d NAME=%s KEY=%s\n", i, name, key)
            fmt.Println(s)
            fmt.Println("---")
          }
        }
      }
    }
  }
}
'''


def run(cmd: list[str], env: dict | None = None, cwd: str | None = None) -> str:
    proc = subprocess.run(cmd, capture_output=True, text=True, env=env, cwd=cwd)
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip())
    return proc.stdout


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Query SLS by custom query or traceID and print responses."
    )
    p.add_argument("--trace-id", help="Trace id shortcut query")
    p.add_argument("--query", help="Custom SLS query string")
    p.add_argument("--profile", choices=["prod", "test"], default="prod")
    p.add_argument(
        "--from",
        dest="from_time",
        help="Start time, supports unix seconds or 'YYYY-MM-DD HH:MM:SS'",
    )
    p.add_argument(
        "--to",
        dest="to_time",
        help="End time, supports unix seconds or 'YYYY-MM-DD HH:MM:SS'",
    )
    p.add_argument("--lines", type=int, default=100)
    p.add_argument("--offset", type=int, default=0)
    p.add_argument("--logstore", help="Override logstore")
    p.add_argument(
        "--resolver",
        default=str(Path(__file__).resolve().parent / "sls_config_resolver.py"),
    )
    return p.parse_args()


def parse_time_arg(raw: str | None, default_ts: int) -> int:
    if not raw:
        return default_ts
    if raw.isdigit():
        return int(raw)
    for fmt in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S"):
        try:
            return int(datetime.strptime(raw, fmt).timestamp())
        except ValueError:
            continue
    raise ValueError(f"invalid time format: {raw}")


def main() -> int:
    args = parse_args()
    if not args.query and not args.trace_id:
        print("Error: either --query or --trace-id must be provided")
        return 1

    now = datetime.now()
    start_of_today = datetime(now.year, now.month, now.day)
    from_sec = parse_time_arg(args.from_time, int(start_of_today.timestamp()))
    to_sec = parse_time_arg(args.to_time, int(now.timestamp()))

    resolver_cmd = [
        "python3",
        args.resolver,
        "--profile",
        args.profile,
        "--format",
        "json",
    ]
    if args.logstore:
        resolver_cmd.extend(["--logstore", args.logstore])
    cfg_text = run(resolver_cmd)
    payload = json.loads(cfg_text)
    cfg = payload["config"]

    go_src = GO_TEMPLATE.replace("__FROM_SEC__", str(from_sec)).replace(
        "__LINES__", str(args.lines)
    ).replace("__TO_SEC__", str(to_sec)).replace("__OFFSET__", str(args.offset))
    with tempfile.TemporaryDirectory() as d:
        temp_dir = Path(d)
        (temp_dir / "go.mod").write_text(TEMP_GO_MOD, encoding="utf-8")
        (temp_dir / "main.go").write_text(go_src, encoding="utf-8")
        env = dict(
            **os.environ,
            SLS_ENDPOINT=cfg.get("Endpoint", ""),
            SLS_ACCESS_KEY_ID=cfg.get("AccessKeyID", ""),
            SLS_ACCESS_KEY_SECRET=cfg.get("AccessKeySecret", ""),
            SLS_PROJECT=cfg.get("Project", ""),
            SLS_LOGSTORE=cfg.get("Logstore", ""),
            TRACE_ID=args.trace_id or "",
            QUERY_TEXT=args.query or "",
        )
        temp_go_env = env | {"GOWORK": "off", "GO111MODULE": "on"}
        run(["go", "mod", "tidy"], env=temp_go_env, cwd=str(temp_dir))
        out = run(["go", "run", "."], env=temp_go_env, cwd=str(temp_dir))
        print(out, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
