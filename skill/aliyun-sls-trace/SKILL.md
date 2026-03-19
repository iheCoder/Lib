---
name: aliyun-sls-trace
description: Work with Alibaba Cloud SLS tracing logs for list/detail/export tasks and trace troubleshooting. Use when requests mention Aliyun/Alibaba Cloud trace logs, SLS log query, 链路追踪, or traceID-based debugging. Resolve config independently from this skill's own profiles file by default, and optionally load from a Go constants file when explicitly provided.
---

# Aliyun SLS Trace

## Overview

Resolve and apply SLS trace configuration from this skill's own profiles file (`references/sls_profiles.json`), with default profile `prod` and optional `test` override. Reuse the bundled resolver/query scripts to avoid hard-coding endpoint/project/logstore/AKSK in each task.

This copy is repo-local so it can move with the repository and still work on another machine without depending on `~/.codex/skills`.

## Workflow

1. Resolve config first with:
```bash
python3 scripts/sls_config_resolver.py --profile prod
```
2. Switch to test explicitly when requested:
```bash
python3 scripts/sls_config_resolver.py --profile test
```
3. Override individual fields only when user asks for custom values:
```bash
python3 scripts/sls_config_resolver.py --profile prod --project custom-project --logstore custom-store
```
4. Only when needed, load config from an external Go constants file:
```bash
python3 scripts/sls_config_resolver.py --profile prod --config-file path/to/sls_test.go
```
5. Query trace response candidates with:
```bash
python3 scripts/query_trace_response.py --trace-id <trace_id> --profile prod
```
6. Query with arbitrary SLS query syntax:
```bash
python3 scripts/query_trace_response.py --query 'level: "error" and "/v1/user/get_knowledge_level_new"' --profile prod
```

## Conventions

- Default profile: `prod` (`ProdSlsConfigParam`).
- Optional profile: `test` (`TestSlsConfigParam`).
- Keep default config source of truth in `references/sls_profiles.json`.
- `--config-file` is optional and only used for explicit external overrides.
- Prefer script output format `json`; use `--format env` only for shell export scenarios.
- `query_trace_response.py` runs in its own temporary Go module, so it should work the same across different repos.

## Script

- `scripts/sls_config_resolver.py`
  - Load `prod/test` from `references/sls_profiles.json` by default.
  - Optionally parse `TestSlsConfigParam` and `ProdSlsConfigParam` from a provided Go file.
  - Output resolved config as JSON or env lines.
  - Support environment variable overrides (`SLS_*`).
  - Support command-line overrides (`--endpoint --ak --sk --project --logstore`).
  - Default to profile `prod`.
- `scripts/query_trace_response.py`
  - Support two modes:
    - `--trace-id` shortcut mode (`traceID` + full-text fallback).
    - `--query` arbitrary SLS query mode (execute query as-is).
  - Run the temporary Go client inside an isolated temp module instead of the caller's repo module.
  - Print candidate payload fields (`response/result/body/output`) from matched logs.
  - Support flexible parameters:
    - `--logstore` to override logstore.
    - `--from` and `--to` for time range (unix seconds or `YYYY-MM-DD HH:MM:SS`).
    - `--lines` and `--offset` for pagination.
  - Defaults:
    - profile `prod`
    - time range `today 00:00:00` to `now`
    - `lines=100`
    - `offset=0`

## Reference

- Detailed examples: `references/examples.md`
