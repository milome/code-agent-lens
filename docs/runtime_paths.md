# CodeAgentLens Runtime Paths

This document is the source of truth for local runtime data paths.

## Canonical Path

CodeAgentLens uses this canonical local data directory on this machine:

```text
D:\DevTools\code-agent-lens\data
```

The canonical SQLite database path is:

```text
D:\DevTools\code-agent-lens\data\code-agent-lens.db
```

The canonical local observability dump path is:

```text
D:\DevTools\code-agent-lens\data\observability
```

Docker maps the canonical directory into the container as:

```text
D:/DevTools/code-agent-lens/data:/data
```

Therefore, inside Docker:

```text
/data/code-agent-lens.db
/data/observability
```

refer to the same host data as the native runtime when native is started with the same data directory.

## Native Runtime

On Windows, native CodeAgentLens defaults to the canonical data directory:

```text
D:\DevTools\code-agent-lens\data
```

Therefore the README native quickstart and the Docker full stack use the same canonical database path unless explicitly overridden.

To override the native data directory, set:

```powershell
$env:CODE_AGENT_LENS_DATA_DIR='D:\DevTools\code-agent-lens\data'
```

Desktop/native builds that expose CLI args may also accept:

```powershell
--data-dir D:\DevTools\code-agent-lens\data
```

Do not use a second native data directory when comparing Docker and native behavior.

## Legacy ccNexus Path

The following path is legacy ccNexus data and is not the CodeAgentLens runtime path:

```text
D:\DevTools\shared\ccnexus
```

It may be used only as a migration source, for example to import old endpoint credentials. It must not be used as the active CodeAgentLens data directory.

## Chinese Summary

本机 CodeAgentLens 唯一规范运行目录是：

```text
D:\DevTools\code-agent-lens\data
```

规范数据库是：

```text
D:\DevTools\code-agent-lens\data\code-agent-lens.db
```

`D:\DevTools\shared\ccnexus` 是旧 ccNexus 数据路径，只能作为迁移来源，不能作为 CodeAgentLens 当前运行目录。
