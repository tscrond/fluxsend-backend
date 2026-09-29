# Prometheus Metrics

FluxSend can expose Prometheus metrics on a dedicated HTTP listener. The listener is disabled by default and is not mounted on the public API router.

## What is exposed

The metrics endpoint exports:

- request volume, latency, in-flight requests, response size, and panic counts for the API, CLI API, and admin server
- Go runtime and process metrics from the standard Prometheus collectors
- build metadata through `fluxsend_build_info`
- storage, email, database health, and database error metrics
- business counters for uploads, shares, downloads, workspaces, invites, members, API keys, auth, and quota rejections
- cached DB-derived gauges for users, plans, files, storage, workspaces, shares, and active API keys

## Configuration

Use either YAML config or environment variables.

```yaml
metrics:
  enabled: true
  listen_port: "9464"
  bind_address: "127.0.0.1"
  path: "/metrics"
```

Environment variables:

- `METRICS_ENABLED`
- `METRICS_LISTEN_PORT`
- `METRICS_BIND_ADDRESS`
- `METRICS_PATH`

Defaults:

- `enabled=false`
- `listen_port=9464`
- `bind_address=""` which means all interfaces
- `path=/metrics`

The same listener is available in both the normal backend mode and `./fluxsend --dev-api` mode when metrics are enabled.

## Local scrape

Example local startup:

```bash
METRICS_ENABLED=true \
METRICS_BIND_ADDRESS=127.0.0.1 \
METRICS_LISTEN_PORT=9464 \
./fluxsend --config ./config.yaml
```

Scrape it directly:

```bash
curl -s http://127.0.0.1:9464/metrics | head -n 20
```

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: fluxsend
    metrics_path: /metrics
    static_configs:
      - targets:
          - 127.0.0.1:9464
```

## Security considerations

- The endpoint is unauthenticated by design.
- The default bind address is all interfaces, which is convenient for containers but broad for bare-metal hosts.
- Prefer `127.0.0.1`, a private interface, a service mesh, or host firewall rules unless Prometheus scrapes over an isolated network.
- Do not publish the metrics listener on the public internet.

## Example PromQL

Request rate by route and status code:

```promql
sum by (route, status_code) (
  rate(fluxsend_http_requests_total[5m])
)
```

P95 request latency by route:

```promql
histogram_quantile(
  0.95,
  sum by (le, route) (
    rate(fluxsend_http_request_duration_seconds_bucket[5m])
  )
)
```

Quota rejections by limit:

```promql
sum by (limit) (
  rate(fluxsend_quota_rejections_total[15m])
)
```

Download volume by channel and mechanism:

```promql
sum by (channel, mechanism, outcome) (
  rate(fluxsend_downloads_total[5m])
)
```

Database availability:

```promql
fluxsend_db_up
```

Active keys by domain:

```promql
fluxsend_active_api_keys
```