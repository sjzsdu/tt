# Agent 命令文档

## 概述

`tt agent` 是统一的 agent 管理命令，集成了原有的 `nvwa` 和 `agent optimize` 功能。

## 命令结构

```
tt agent [message]                    # 运行 agent（交互或单次消息）
tt agent list                         # 列出所有 agents
tt agent info                         # 显示 agent 运行时信息
tt agent create                       # 创建新 agent
tt agent optimize                     # 优化现有 agent
```

## 子命令详解

### tt agent [message]

运行嵌入的 picoclaw agent。

```bash
tt agent -m "summarize this project"
tt agent "explain the current directory"
tt agent --list
tt agent --session cli:tt --model gpt-5.4 -m "review this idea"
```

### tt agent list

列出所有可用的 agents（嵌入式和 picoclaw 配置的）。

```bash
tt agent list
```

### tt agent info

显示 agent 运行时的详细信息。

```bash
tt agent info
```

### tt agent create

创建新的嵌入式 agent。

```bash
# 从角色描述创建
tt agent create --role "前端开发工程师"
tt agent create --role "产品经理" --context "偏增长型 SaaS"
tt agent create --role "Go 后端工程师" --output .agents/go-backend

# 从建议创建
tt agent create --name coder --suggestion "更注重性能优化"
```

**参数说明：**

| 参数 | 说明 |
|------|------|
| `--role` | 角色描述，用于从零创建新 agent |
| `--name` | agent 名称，配合 `--suggestion` 使用 |
| `--suggestion` | 优化建议 |
| `--output, -o` | 输出目录（默认 `.tt/agents`） |
| `--skill` | agent 技能（可多次使用） |
| `--no-history` | 禁用对话历史 |
| `--research-tools` | 包含研究工具 |
| `--model` | 指定模型 |
| `--session, -s` | 会话标识 |
| `--debug, -d` | 启用调试日志 |
| `--picoclaw-home` | 覆盖 PICOCLAW_HOME |
| `--picoclaw-config` | 覆盖 PICOCLAW_CONFIG |
| `--force` | 覆盖已有文件 |

### tt agent optimize

优化现有 agent。

```bash
# 基于仓库知识蒸馏
tt agent optimize --agent coder --target ./repo
tt agent optimize --agent coder --target github.com/gin-gonic/gin --copy

# 基于自然语言建议
tt agent optimize --agent coder --suggestion "更注重性能优化"

# 结合两者
tt agent optimize --agent coder --target ./repo --suggestion "关注安全方面"
```

**参数说明：**

| 参数 | 说明 |
|------|------|
| `--agent` | 基础 agent ID 或本地 .md 文件路径（必需） |
| `--target` | 目标仓库路径或可克隆的 URL |
| `--suggestion` | 自然语言优化建议 |
| `--output, -o` | 输出文件或目录 |
| `--force, -f` | 覆盖已有文件 |
| `--copy` | 创建新的优化 agent，而非原地更新 |
| `--session` | 会话标识（默认 `cli:agent-optimize`） |
| `--model` | 模型覆盖 |
| `--debug, -d` | 启用调试日志 |
| `--max-files` | 最大收集文件数（默认 200） |
| `--max-file-size` | 最大文件大小（默认 256KB） |
| `--max-prompt-chars` | 最大 prompt 字符数（默认 12000） |
| `--timeout` | 超时时间（默认 2 分钟） |
| `--keep-temp` | 保留临时克隆的仓库 |

## 使用示例

### 创建前端工程师 agent

```bash
tt agent create --role "前端开发工程师" --skill browser --skill react
```

### 为现有 agent 添加仓库知识

```bash
tt agent optimize --agent frontend --target ./my-react-app
```

### 基于建议优化 agent

```bash
tt agent optimize --agent frontend --suggestion "更注重移动端适配和性能优化"
```

### 交互式运行 agent

```bash
tt agent
> help me review this code
```

## 向后兼容性

- 原 `tt nvwa` 命令已删除，功能迁移到 `tt agent create`
- 原 `tt agent optimize --target` 功能保持不变
- 新增 `--suggestion` 选项支持基于建议的优化
