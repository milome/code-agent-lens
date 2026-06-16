# 详细配置

## 应用设置

| 设置项 | 说明 | 默认值 |
|--------|------|--------|
| 代理端口 | 本地代理监听端口 | `3000` |
| 日志级别 | 0= 调试，1= 信息，2= 警告，3= 错误 | `1` |
| 界面语言 | 中文 / English | `zh-CN` |
| 主题 | 12 种主题可选 | `light` |
| 自动主题 | 根据时间自动切换（7:00-19:00 浅色） | 关闭 |
| 窗口关闭行为 | 直接关闭 / 最小化到托盘 / 每次询问 | 每次询问 |

## 端点配置

### 转换器类型

| 转换器 | 说明 |
|--------|------|
| `claude` | Claude API |
| `openai` | OpenAI Chat API |
| `openai2` | OpenAI Response API |
| `gemini` | Google Gemini API |

### 配置示例

**Claude 端点：**
```json
{
  "name": "Claude 官方",
  "apiUrl": "https://api.anthropic.com",
  "apiKey": "sk-ant-api03-xxx",
  "enabled": true,
  "transformer": "claude"
}
```

**OpenAI 端点：**
```json
{
  "name": "OpenAI 代理",
  "apiUrl": "https://api.openai.com",
  "apiKey": "sk-xxx",
  "enabled": true,
  "transformer": "openai",
  "model": "gpt-4-turbo"
}
```

**Gemini 端点：**
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

## v1.0.0 备份方式

首个公开版本将配置和统计数据保存在本地 SQLite 数据库中。需要备份时，直接复制数据库文件。

云端备份提供商不属于 v1.0.0 支持合同范围。

## 数据存储位置

- 本机规范数据目录：`D:\DevTools\code-agent-lens\data`
- 数据库：`D:\DevTools\code-agent-lens\data\code-agent-lens.db`
- Observability dump：`D:\DevTools\code-agent-lens\data\observability`

Docker 将 `D:\DevTools\code-agent-lens\data` 映射到容器内 `/data`，因此容器内 `/data/code-agent-lens.db` 与 native 使用同一个宿主数据库。

`D:\DevTools\shared\ccnexus` 是旧 ccNexus 迁移来源，不是 CodeAgentLens 当前运行目录。
