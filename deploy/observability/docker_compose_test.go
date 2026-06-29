package observability

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const sharedDevToolsDataMount = `"D:/DevTools/code-agent-lens/data:/data"`

func TestFullComposeUsesSharedDevToolsDataBindMount(t *testing.T) {
	body := readTextFile(t, "docker-compose.full.yaml")

	assertSharedDataBindMount(t, body)
	for _, forbidden := range []string{
		"code-agent-lens-data:/data",
		"code-agent-lens-observability:/data/observability",
		"code-agent-lens-data:",
		"code-agent-lens-observability:",
		"D:/DevTools/shared/ccnexus",
		"D:\\DevTools\\shared\\ccnexus",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("docker-compose.full.yaml must not use named app data volume %q", forbidden)
		}
	}
}

func TestServerComposeUsesSharedDevToolsDataBindMount(t *testing.T) {
	body := readTextFile(t, "../../cmd/server/docker-compose.yml")

	assertSharedDataBindMount(t, body)
	if strings.Contains(body, "./code-agent-lens/:/data") {
		t.Fatalf("cmd/server/docker-compose.yml must not use repo-local app data bind mount")
	}
	if strings.Contains(body, "../../.tmp/docker-observability:/data/observability") {
		t.Fatalf("cmd/server/docker-compose.yml must not split observability dumps into .tmp")
	}
	for _, forbidden := range []string{
		"D:/DevTools/shared/ccnexus",
		"D:\\DevTools\\shared\\ccnexus",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("cmd/server/docker-compose.yml must not use legacy ccNexus path %q", forbidden)
		}
	}
}

func TestCollectorLocalConfigIncludesLogsPipeline(t *testing.T) {
	body := readTextFile(t, "config/otel/collector.local.yaml")

	for _, want := range []string{
		"  debug:\n    verbosity: detailed",
		"  transform/logs_readable_body:",
		"set(body, attributes[\"event.name\"]) where (body == nil or body == \"\") and attributes[\"event.name\"] != nil",
		"set(body, Concat([body, attributes[\"event.kind\"]], \" \")) where body != nil and body != \"\" and attributes[\"event.kind\"] != nil",
		"set(body, Concat([body, attributes[\"tool_name\"]], \" \")) where body != nil and body != \"\" and attributes[\"tool_name\"] != nil",
		"set(body, Concat([body, attributes[\"model\"]], \" \")) where body != nil and body != \"\" and attributes[\"model\"] != nil",
		"  otlphttp/logs:\n    endpoint: http://loki:3100/otlp",
		"    logs:\n      receivers:\n        - otlp\n      processors:\n        - batch\n        - transform/logs_readable_body\n      exporters:\n        - otlphttp/logs\n        - debug",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("collector.local.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestObservabilityComposeIncludesQueryableLogsStack(t *testing.T) {
	body := readTextFile(t, "docker-compose.observability.yaml")

	for _, want := range []string{
		"  loki:",
		"image: grafana/loki:",
		"- \"-config.file=/etc/loki/loki.yaml\"",
		"- \"127.0.0.1:3100:3100\"",
		"- ./config/loki/loki.yaml:/etc/loki/loki.yaml:ro",
		"- loki-data:/loki",
		"- loki",
		"  loki-data:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docker-compose.observability.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestFullComposeExtendsLokiService(t *testing.T) {
	body := readTextFile(t, "docker-compose.full.yaml")

	for _, want := range []string{
		"  loki:\n    extends:\n      file: docker-compose.observability.yaml\n      service: loki",
		"  loki-data:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docker-compose.full.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestLokiLocalConfigSupportsOTLPAndPersistentFilesystemStorage(t *testing.T) {
	body := readTextFile(t, "config/loki/loki.yaml")

	for _, want := range []string{
		"auth_enabled: false",
		"http_listen_port: 3100",
		"path_prefix: /loki",
		"schema: v13",
		"object_store: filesystem",
		"directory: /loki/chunks",
		"allow_structured_metadata: true",
		"volume_enabled: true",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("loki.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestGrafanaDatasourcesIncludeLoki(t *testing.T) {
	body := readTextFile(t, "config/grafana/provisioning/datasources/datasources.yaml")

	for _, want := range []string{
		"- name: Loki",
		"type: loki",
		"url: http://loki:3100",
		"uid: loki",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("datasources.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestObservabilityQuickstartExplainsLogsQueryPath(t *testing.T) {
	body := readTextFile(t, "../../docs/observability/quickstart.md")

	for _, want := range []string{
		"Loki: `http://127.0.0.1:3100/ready`",
		"Grafana Explore",
		"Loki",
		"{service_name=\"code-agent-lens\"}",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("quickstart.md missing %q:\n%s", want, body)
		}
	}
}

func TestReusableOTELStackFilesExist(t *testing.T) {
	for _, path := range []string{
		"../otel-stack/docker-compose.yaml",
		"../otel-stack/.env.example",
		"../otel-stack/README.md",
		"../otel-stack/config/otel/collector.yaml",
		"../otel-stack/config/grafana/provisioning/datasources/datasources.yaml",
		"../otel-stack/config/prometheus/prometheus.yaml",
		"../otel-stack/config/tempo/tempo.yaml",
		"../otel-stack/config/loki/loki.yaml",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("reusable otel stack file missing %s: %v", path, err)
		}
	}
}

func TestReusableOTELStackEnvExampleDefinesPortsAndGrafanaSettings(t *testing.T) {
	body := readTextFile(t, "../otel-stack/.env.example")

	for _, want := range []string{
		"OTEL_BIND_HOST=127.0.0.1",
		"OTEL_GRPC_PORT=4317",
		"OTEL_HTTP_PORT=4318",
		"GRAFANA_PORT=13000",
		"LOKI_PORT=3100",
		"PROMETHEUS_PORT=9090",
		"TEMPO_PORT=3200",
		"JAEGER_PORT=16686",
		"OTEL_COLLECTOR_METRICS_PORT=8888",
		"GF_SECURITY_ALLOW_EMBEDDING=true",
		"GF_AUTH_ANONYMOUS_ENABLED=true",
		"GF_AUTH_ANONYMOUS_ORG_ROLE=Admin",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf(".env.example missing %q:\n%s", want, body)
		}
	}
}

func TestReusableOTELStackComposeDefinesGenericServicesAndVolumes(t *testing.T) {
	body := readTextFile(t, "../otel-stack/docker-compose.yaml")

	for _, want := range []string{
		"  otel-collector:",
		"  grafana:",
		"  prometheus:",
		"  loki:",
		"  tempo:",
		"  jaeger:",
		"${OTEL_BIND_HOST}:${OTEL_GRPC_PORT}:4317",
		"${OTEL_BIND_HOST}:${OTEL_HTTP_PORT}:4318",
		"${OTEL_BIND_HOST}:${GRAFANA_PORT}:3000",
		"${OTEL_BIND_HOST}:${LOKI_PORT}:3100",
		"${OTEL_BIND_HOST}:${PROMETHEUS_PORT}:9090",
		"${OTEL_BIND_HOST}:${TEMPO_PORT}:3200",
		"${OTEL_BIND_HOST}:${JAEGER_PORT}:16686",
		"${OTEL_BIND_HOST}:${OTEL_COLLECTOR_METRICS_PORT}:8888",
		"- loki-data:/loki",
		"  grafana-data:",
		"  prometheus-data:",
		"  loki-data:",
		"  tempo-data:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("otel-stack docker-compose.yaml missing %q:\n%s", want, body)
		}
	}

	for _, forbidden := range []string{
		"127.0.0.1:4317:4317",
		"127.0.0.1:4318:4318",
		"127.0.0.1:13000:3000",
		"127.0.0.1:3100:3100",
		"127.0.0.1:9090:9090",
		"127.0.0.1:3200:3200",
		"127.0.0.1:16686:16686",
		":14250:14250",
		":8888:8888",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("otel-stack docker-compose.yaml must not hard-code host port mapping %q", forbidden)
		}
	}
}

func TestReusableOTELStackCollectorDefinesTracesMetricsAndLogs(t *testing.T) {
	body := readTextFile(t, "../otel-stack/config/otel/collector.yaml")

	for _, want := range []string{
		"  otlp:",
		"  batch: {}",
		"  transform/logs_readable_body:",
		"set(body, attributes[\"event.name\"]) where (body == nil or body == \"\") and attributes[\"event.name\"] != nil",
		"set(body, Concat([body, attributes[\"event.kind\"]], \" \")) where body != nil and body != \"\" and attributes[\"event.kind\"] != nil",
		"set(body, Concat([body, attributes[\"tool_name\"]], \" \")) where body != nil and body != \"\" and attributes[\"tool_name\"] != nil",
		"set(body, Concat([body, attributes[\"model\"]], \" \")) where body != nil and body != \"\" and attributes[\"model\"] != nil",
		"  otlp/jaeger:",
		"    endpoint: jaeger:4317",
		"  otlp/tempo:",
		"    endpoint: tempo:4317",
		"  prometheus:",
		"    endpoint: 0.0.0.0:8888",
		"  otlphttp/logs:",
		"    endpoint: http://loki:3100/otlp",
		"  debug:",
		"    traces:",
		"    metrics:",
		"    logs:",
		"        - transform/logs_readable_body",
		"        - otlphttp/logs",
		"        - debug",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("otel-stack collector.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestReusableOTELStackGrafanaDatasourcesIncludeTempoPrometheusAndLoki(t *testing.T) {
	body := readTextFile(t, "../otel-stack/config/grafana/provisioning/datasources/datasources.yaml")

	for _, want := range []string{
		"- name: Tempo",
		"type: tempo",
		"url: http://tempo:3200",
		"uid: tempo",
		"- name: Prometheus",
		"type: prometheus",
		"url: http://prometheus:9090",
		"uid: prometheus",
		"- name: Loki",
		"type: loki",
		"url: http://loki:3100",
		"uid: loki",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("otel-stack datasources.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestReusableOTELStackLokiConfigUsesPersistentFilesystemStorage(t *testing.T) {
	body := readTextFile(t, "../otel-stack/config/loki/loki.yaml")

	for _, want := range []string{
		"auth_enabled: false",
		"path_prefix: /loki",
		"object_store: filesystem",
		"schema: v13",
		"allow_structured_metadata: true",
		"volume_enabled: true",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("otel-stack loki.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestReusableOTELStackReadmeIsGenericAndExplainsOTLPOnboarding(t *testing.T) {
	body := readTextFile(t, "../otel-stack/README.md")

	for _, want := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318",
		"OTEL_SERVICE_NAME=my-project",
		"OTEL_TRACES_EXPORTER=otlp",
		"OTEL_METRICS_EXPORTER=otlp",
		"OTEL_LOGS_EXPORTER=otlp",
		"Grafana Explore",
		"{service_name=\"my-project\"}",
		"docker compose --env-file .env.example -f docker-compose.yaml config",
		"docker compose --env-file .env.example -f docker-compose.yaml up -d",
		"curl http://127.0.0.1:3100/ready",
		"curl http://127.0.0.1:13000/api/health",
		"docker compose --env-file .env.example -f docker-compose.yaml down",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("otel-stack README.md missing %q:\n%s", want, body)
		}
	}

	for _, forbidden := range []string{
		"CodeAgentLens",
		"Debug Portal",
		"code-agent-lens_obs_ref",
		"D:/DevTools/code-agent-lens",
		"/debug/obs",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("otel-stack README.md must not contain product-specific term %q", forbidden)
		}
	}
}

func TestReusableOTELStackComposeConfigRenders(t *testing.T) {
	cmd := exec.Command("docker", "compose", "--env-file", "../otel-stack/.env.example", "-f", "../otel-stack/docker-compose.yaml", "config")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, string(out))
	}
}

func assertSharedDataBindMount(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{
		sharedDevToolsDataMount,
		"CODE_AGENT_LENS_DATA_DIR",
		"/data",
		"CODE_AGENT_LENS_DB_PATH",
		"/data/code-agent-lens.db",
		"CODE_AGENT_LENS_OBS_DUMP_DIR",
		"/data/observability",
		"CODE_AGENT_LENS_OBS_SMOKE_UPSTREAM",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("compose file missing %q:\n%s", want, body)
		}
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
