#!/usr/bin/env node
import fs from "node:fs";

const checks = [
  ["docs/README_DOCKER.md", ["code-agent-lens_obs_ref", "Trace correlation", "traceID", "spanID", "CODE_AGENT_LENS_OBS_VIEWER_PUBLIC_URL", "http://127.0.0.1:3011/debug/obs", "/debug/obs/tool/jaeger", "/debug/obs/tool/grafana", "GF_SECURITY_ALLOW_EMBEDDING=true", "otlphttp/logs", "Loki", "Grafana Explore", "{service_name=\"code-agent-lens\"}"]],
  ["docs/observability/jaeger-ui-config.json", ["code-agent-lens_obs_ref", "linkPatterns", "http://127.0.0.1:3011/debug/obs"]],
  ["docs/observability/otel-collector-local.yaml", ["Trace correlation", "traceID", "spanID", "code-agent-lens_obs_ref", "http://127.0.0.1:3011/debug/obs", "otlphttp/logs", "http://127.0.0.1:3100/otlp", "logs:", "exporters: [otlphttp/logs, debug]"]],
  ["docs/observability/quickstart.md", ["Loki: `http://127.0.0.1:3100/ready`", "Grafana Explore", "Loki", "{service_name=\"code-agent-lens\"}", "http://loki:3100/otlp"]],
  ["docs/observability/claude-code-settings.example.json", ["CLAUDE_CODE_ENABLE_TELEMETRY", "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA", "CLAUDE_CODE_PROPAGATE_TRACEPARENT", "OTEL_LOG_RAW_API_BODIES", "CODE_AGENT_LENS_OBS_VIEWER_PUBLIC_URL", "http://127.0.0.1:3011/debug/obs"]],
  ["deploy/otel-stack/README.md", ["OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318", "OTEL_SERVICE_NAME=my-project", "OTEL_TRACES_EXPORTER=otlp", "OTEL_METRICS_EXPORTER=otlp", "OTEL_LOGS_EXPORTER=otlp", "Grafana Explore", "Loki", "{service_name=\"my-project\"}", "docker compose --env-file .env.example -f docker-compose.yaml config"]],
];

const forbiddenChecks = [
  ["deploy/otel-stack/README.md", ["CodeAgentLens", "Debug Portal", "code-agent-lens_obs_ref", "D:/DevTools/code-agent-lens", "/debug/obs"]],
];

const failures = [];
for (const [file, literals] of checks) {
  if (!fs.existsSync(file)) {
    failures.push(`${file}: missing`);
    continue;
  }
  const text = fs.readFileSync(file, "utf8");
  for (const literal of literals) {
    if (!text.includes(literal)) {
      failures.push(`${file}: missing literal ${literal}`);
    }
  }
}

for (const [file, literals] of forbiddenChecks) {
  if (!fs.existsSync(file)) {
    failures.push(`${file}: missing`);
    continue;
  }
  const text = fs.readFileSync(file, "utf8");
  for (const literal of literals) {
    if (text.includes(literal)) {
      failures.push(`${file}: forbidden literal ${literal}`);
    }
  }
}

if (failures.length > 0) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log(`observability docs checks passed files=${checks.length}`);
