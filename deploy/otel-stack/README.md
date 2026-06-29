# Reusable OTEL Stack

This directory provides a local OpenTelemetry stack for any project that can export OTLP telemetry.

## Start

```powershell
docker compose --env-file .env.example -f docker-compose.yaml config
docker compose --env-file .env.example -f docker-compose.yaml up -d
```

## Application Configuration

Use these environment variables in the application that emits telemetry:

```env
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
OTEL_SERVICE_NAME=my-project
OTEL_TRACES_EXPORTER=otlp
OTEL_METRICS_EXPORTER=otlp
OTEL_LOGS_EXPORTER=otlp
```

Use `http://otel-collector:4318` instead of `http://127.0.0.1:4318` for workloads running in the same compose network as this stack.

## Query Telemetry

- Traces: open Grafana and use the Tempo datasource, or open Jaeger.
- Metrics: open Grafana and use the Prometheus datasource.
- Logs: open Grafana Explore, select the Loki datasource, and query `{service_name="my-project"}`.

## Default Local Endpoints

- OTLP gRPC: `http://127.0.0.1:4317`
- OTLP HTTP: `http://127.0.0.1:4318`
- Grafana: `http://127.0.0.1:13000`
- Loki readiness: `http://127.0.0.1:3100/ready`
- Prometheus: `http://127.0.0.1:9090/graph`
- Tempo status: `http://127.0.0.1:3200/status`
- Jaeger: `http://127.0.0.1:16686`
- Collector metrics: `http://127.0.0.1:8888/metrics`

## Smoke Test

```powershell
docker compose --env-file .env.example -f docker-compose.yaml config
docker compose --env-file .env.example -f docker-compose.yaml up -d
curl http://127.0.0.1:3100/ready
curl http://127.0.0.1:13000/api/health
docker compose --env-file .env.example -f docker-compose.yaml down
```

Verify logs in Grafana Explore with:

```logql
{service_name="my-project"}
```
