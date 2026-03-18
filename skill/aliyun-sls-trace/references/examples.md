# Aliyun SLS Trace Examples

## Export with default prod profile

```bash
python3 scripts/sls_config_resolver.py
```

## Switch to test profile

```bash
python3 scripts/sls_config_resolver.py --profile test
```

## Use env format

```bash
python3 scripts/sls_config_resolver.py --format env
```

## Override project/logstore

```bash
python3 scripts/sls_config_resolver.py --project custom-project --logstore custom-logstore
```

## Optional: load from external Go constants

```bash
python3 scripts/sls_config_resolver.py --config-file /path/to/mcp/sls/sls_test.go --profile prod
```

## Query trace response candidates

```bash
python3 scripts/query_trace_response.py --trace-id 95d2d87ffb0980d8876f73f8bf06b1dd --profile prod
```

## Query with arbitrary SLS query

```bash
python3 scripts/query_trace_response.py \
  --query '"/v1/user/get_knowledge_level_new" and level: "info"' \
  --profile prod
```

## Query with custom time range and pagination

```bash
python3 scripts/query_trace_response.py \
  --trace-id 95d2d87ffb0980d8876f73f8bf06b1dd \
  --profile prod \
  --from "2026-03-01 00:00:00" \
  --to "2026-03-03 23:59:59" \
  --lines 200 \
  --offset 0
```

## Query with custom logstore

```bash
python3 scripts/query_trace_response.py \
  --trace-id 95d2d87ffb0980d8876f73f8bf06b1dd \
  --profile prod \
  --logstore data-engine-service-gray
```

The first run may download the Go SDK in the temp module. That is expected; it should not depend on the current repo's `go.mod`.
