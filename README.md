# mini-ssp

A minimal but production-grade Supply-Side Platform (SSP) written in Go.

Accepts a bid request, fans it out to multiple DSPs in parallel over gRPC, runs a second-price (Vickrey) auction, applies frequency capping, and streams auction events to ClickHouse for analytics.

## Architecture

```
Client
  │
  ▼ POST /bid
┌─────────────────────────────────────────┐
│                  SSP                    │
│  rate limit → freqcap → auction → win   │
└──┬──────────────────────────────────┬───┘
   │ gRPC (parallel, 150ms timeout)   │ fire-and-forget
   ▼                                  ▼
DSP-1 … DSP-6                       Kafka
   │                                  │
Redis (atomic freqcap)            Consumer
   │                                  │
PostgreSQL (DSP config,           ClickHouse
 per-advertiser rules)               │
                                  Prometheus → Grafana
```

## Features

- **Second-price auction** — Vickrey auction with configurable floor price
- **Parallel DSP calls** — all DSPs contacted concurrently with a 150ms deadline
- **Circuit breaker** — per-DSP breaker opens after 5 consecutive failures, recovers after 30s
- **Frequency capping** — Redis-backed with atomic Lua script (no TOCTOU race); per-advertiser rules loaded from PostgreSQL
- **Kafka streaming** — auction events published fire-and-forget with backpressure (256-slot semaphore)
- **ClickHouse analytics** — batched inserts (1000 events or 5s), MergeTree table
- **Rate limiting** — token bucket on `/bid` via `golang.org/x/time/rate`
- **Graceful shutdown** — 10s drain on SIGINT/SIGTERM
- **Container-aware** — `automaxprocs` sets `GOMAXPROCS` from cgroup CPU quota

## API

```
POST /bid
GET  /metrics   — Prometheus metrics
GET  /healthz   — liveness/readiness probe
```

Full spec: [`openapi.yaml`](openapi.yaml)

### Bid request

```json
{
  "user_id":    "u42",
  "geo":        "US",
  "format":     "banner",
  "floor_price": 0.5
}
```

### Bid response

```json
{
  "advertiser_id": "adcorp",
  "clearing_price": 1.35
}
```

Returns `204 No Content` when no DSP bids above the floor or the user is frequency-capped.

## Quick start

```bash
docker compose up --build
```

Services:

| Service     | URL                        |
|-------------|----------------------------|
| SSP         | http://localhost:8080      |
| Grafana     | http://localhost:3000      |
| Prometheus  | http://localhost:9090      |
| ClickHouse  | localhost:9000 (native)    |

Send a test bid:

```bash
curl -s -X POST http://localhost:8080/bid \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"u1","geo":"US","format":"banner","floor_price":0.5}' | jq
```

## Configuration

All flags with defaults:

| Flag               | Default           | Description                                  |
|--------------------|-------------------|----------------------------------------------|
| `-port`            | `8080`            | HTTP listen port                             |
| `-dsps`            | —                 | Comma-separated DSP gRPC addresses           |
| `-postgres`        | —                 | PostgreSQL DSN (overrides `-dsps`)           |
| `-redis`           | —                 | Redis address for frequency capping          |
| `-freqcap-limit`   | `3`               | Max impressions per user per window          |
| `-freqcap-window`  | `1h`              | Frequency cap window duration                |
| `-kafka`           | —                 | Comma-separated Kafka brokers                |
| `-kafka-topic`     | `auction.events`  | Kafka topic for auction events               |
| `-rate-limit`      | `0` (disabled)    | Max requests/sec on `/bid`                   |
| `-rate-burst`      | `10`              | Burst size for rate limiter                  |

## Project layout

```
cmd/
  ssp/        — main SSP binary
  dsp/        — mock DSP server (gRPC)
  consumer/   — Kafka → ClickHouse consumer
internal/
  auction/    — Vickrey auction logic
  dsp/        — DSP interface, gRPC client with circuit breaker
  events/     — Kafka publisher with backpressure
  freqcap/    — Redis frequency capper (atomic Lua)
  middleware/ — rate limiting
  metrics/    — Prometheus metric definitions
  ssp/        — HTTP handler
  store/      — PostgreSQL store (pgxpool)
k8s/          — Kubernetes manifests (13 files)
helm/         — Helm chart
.github/      — CI/CD (test, lint, docker build)
```

## Kubernetes

### Raw manifests

```bash
kubectl apply -f k8s/
```

### Helm

```bash
helm install mini-ssp ./helm/mini-ssp \
  --set ingress.enabled=true \
  --set ingress.host=ssp.example.com \
  --set ingress.tls=true \
  --set ingress.tlsSecret=ssp-tls \
  --set prometheusRule.enabled=true
```

Key values:

```yaml
ssp:
  replicas: 2
  rateLimit: 1000      # rps, 0 = disabled

freqcap:
  limit: 3
  window: 1h

ingress:
  enabled: false
  className: nginx
  host: ssp.local
  grafanaHost: grafana.local
  tls: false

prometheusRule:
  enabled: false
  p99ThresholdSeconds: 0.2
  noBidRateThreshold: 0.5
```

## Metrics

| Metric                              | Type      | Description                        |
|-------------------------------------|-----------|------------------------------------|
| `ssp_bids_total`                    | counter   | Bid outcomes by `result` label     |
| `ssp_auction_duration_seconds`      | histogram | End-to-end auction latency         |
| `ssp_kafka_publish_errors_total`    | counter   | Kafka publish failures             |
| `consumer_events_processed_total`   | counter   | Events written to ClickHouse       |
| `consumer_batches_dropped_total`    | counter   | Batches dropped after retries      |
| `consumer_batch_size`               | histogram | Events per ClickHouse insert batch |
| `consumer_flush_duration_seconds`   | histogram | ClickHouse insert latency          |

SLA alerts (requires Prometheus Operator):
- `SSPHighP99Latency` — p99 > 200ms for 2m
- `SSPHighErrorRate` — no-bid rate > 50% for 5m
- `SSPKafkaErrors` — Kafka errors > 1/s for 5m

## Testing

```bash
go test ./...
```

- `internal/freqcap` — 9 unit tests with miniredis (including concurrent correctness)
- `internal/store` — 3 integration tests with testcontainers (real PostgreSQL)
- `internal/ssp` — handler tests (freqcap block, auction win, no bids)

## Tech stack

Go · gRPC · Redis · Kafka · ClickHouse · PostgreSQL · Prometheus · Grafana · Docker · Kubernetes · Helm · GitHub Actions
