# CodeAgentLens observability quickstart

This guide describes the source-controlled local observability stack for CodeAgentLens.

## Default entry points

Gateway/API: `http://127.0.0.1:3010`
Debug Portal: `http://127.0.0.1:3011/debug/obs`
Jaeger: `http://127.0.0.1:16686`
Grafana: `http://127.0.0.1:13000`
Prometheus: `http://127.0.0.1:9090/graph`
Tempo status: `http://127.0.0.1:3200/status`
OTel Collector: `http://127.0.0.1:8888/metrics`

## Source checkout path

```powershell
git clone https://github.com/<frozen-owner>/code-agent-lens.git
cd code-agent-lens
go run ./cmd/code-agent-lens obs validate --deployment-profile lan_team --simulate-network --profile deploy/observability/stack.local.yaml --evidence-dir .tmp/release-gate/deployment-sim/lan_team
go run ./cmd/code-agent-lens obs validate --deployment-profile public_server --simulate-proxy --profile deploy/observability/stack.local.yaml --evidence-dir .tmp/release-gate/deployment-sim/public_server
```

## Stack profile contract

`deploy/observability/stack.local.yaml` is the default source for runtime modes, tool URLs, capture defaults, and deployment security profiles.

`local_debug` is the only profile that permits disabled portal auth, and only with loopback bind. `lan_team` and `public_server` fail closed unless auth, RBAC, capture policy, and tool exposure rules pass simulation.

## Agent configuration dry run

```powershell
go run ./cmd/code-agent-lens obs configure-agents --profile deploy/observability/stack.local.yaml --agents claude,codex --backup --evidence-dir .tmp/release-gate/agentconfig
```

The dry run writes the Gateway/API URL `http://127.0.0.1:3010` into preview evidence only. It does not modify user configuration.
