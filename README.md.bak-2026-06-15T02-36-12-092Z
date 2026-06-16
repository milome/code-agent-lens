# CodeAgentLens

A locally running model-endpoint gateway and observability workbench for coding agents.

This project is an independently maintained derivative of ccNexus.

Upstream: https://github.com/lich0821/ccNexus

## What CodeAgentLens provides

CodeAgentLens runs a local gateway for model endpoints and a local observability workbench for coding agents. It focuses on request routing, endpoint decisions, trace inspection, token usage, logs, and local debug artifacts.

## Quickstart

Run the source checkout static validation:

```powershell
go run ./cmd/code-agent-lens obs validate --deployment-profile local_debug --profile deploy/observability/stack.local.yaml --evidence-dir .tmp/release-gate/observability/native
```

Full-chain synthetic trace validation requires a running local runtime. Start it first, then run `obs validate --synthetic --trace`.

Gateway/API default URL: `http://127.0.0.1:3010`.

Debug Portal default URL: `http://127.0.0.1:3011/debug/obs`.

## Maintainer

Maintainer: milome

Security reports: GitHub private vulnerability reporting: https://github.com/milome/code-agent-lens/security/advisories/new
