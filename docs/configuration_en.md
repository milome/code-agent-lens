# Configuration Guide

## Application Settings

| Setting | Description | Default |
|---------|-------------|---------|
| Proxy Port | Local proxy listening port | `3000` |
| Log Level | 0=Debug, 1=Info, 2=Warn, 3=Error | `1` |
| Language | Chinese / English | `zh-CN` |
| Theme | 12 themes available | `light` |
| Auto Theme | Auto switch based on time (7:00-19:00 light) | Off |
| Window Close Behavior | Close / Minimize to tray / Ask every time | Ask every time |

## Endpoint Configuration

### Transformer Types

| Transformer | Description |
|--------|------|
| `claude` | Claude API |
| `openai` | OpenAI Chat API |
| `openai2` | OpenAI Response API |
| `gemini` | Google Gemini API |

### Configuration Examples

**Claude Endpoint:**
```json
{
  "name": "Claude Official",
  "apiUrl": "https://api.anthropic.com",
  "apiKey": "sk-ant-api03-xxx",
  "enabled": true,
  "transformer": "claude"
}
```

**OpenAI Endpoint:**
```json
{
  "name": "OpenAI Proxy",
  "apiUrl": "https://api.openai.com",
  "apiKey": "sk-xxx",
  "enabled": true,
  "transformer": "openai",
  "model": "gpt-4-turbo"
}
```

**Gemini Endpoint:**
```json
{
  "name": "Gemini",
  "apiUrl": "https://generativelanguage.googleapis.com",
  "apiKey": "AIza-xxx",
  "enabled": true,
  "transformer": "gemini",
  "model": "gemini-pro"
}
```

## Backup For v1.0.0

The first public release stores configuration and statistics in the local SQLite database. Back up the database file directly when you need a copy.

Cloud backup providers are deferred from the first public release and are not part of the v1.0.0 support contract.

## Data Storage Location

- Canonical local data directory: `D:\DevTools\code-agent-lens\data`
- Database: `D:\DevTools\code-agent-lens\data\code-agent-lens.db`
- Observability dump: `D:\DevTools\code-agent-lens\data\observability`

Docker maps `D:\DevTools\code-agent-lens\data` to `/data`, so `/data/code-agent-lens.db` inside Docker is the same host database used by native when native is started with the canonical data directory.

`D:\DevTools\shared\ccnexus` is a legacy ccNexus migration source, not the active CodeAgentLens runtime directory.
