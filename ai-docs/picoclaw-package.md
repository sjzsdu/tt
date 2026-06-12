# internal/picoclaw 包文档

## 概述

`internal/picoclaw` 是 `tt` 项目中用于集成 [picoclaw](https://github.com/sipeed/picoclaw) AI 代理框架的封装层。它提供了运行时管理、代理配置、直接/交互式对话、图像生成等功能。

## 架构

```
internal/picoclaw/
├── agent.go           # 代理运行入口（Run/交互模式）
├── constants.go       # 常量定义
├── direct.go          # DirectRunner - 单次请求-响应模式
├── embedded_agent.go  # 嵌入式代理配置与注册
├── image.go           # 图像生成
├── init.go            # 运行时初始化
├── resolve.go         # 选项解析与验证
├── response.go        # 响应处理与空响应恢复
├── runtime.go         # Runtime 结构体与加载
└── summary.go         # 状态摘要与模型解析
```

## 核心组件

### 1. Runtime

`Runtime` 是核心结构体，管理 picoclaw 的生命周期：

```go
type Runtime struct {
    Home       string              // picoclaw 主目录
    ConfigPath string              // 配置文件路径
    Config     *pcconfig.Config    // 加载的配置
    Skills     []pcskills.SkillInfo // 可用技能列表
    TTConfig   ttconfig.Config     // tt 配置
    TTSources  ttconfig.Sources    // tt 配置源
}
```

**加载方式：**
```go
rt, err := picoclaw.Load(picoclaw.Options{
    Home:      "/path/to/home",
    Config:    "/path/to/config.json",
    TTConfig:  ttConfig,
    TTSources: ttSources,
})
```

### 2. RunOptions

运行选项配置：

```go
type RunOptions struct {
    Message        string           // 用户消息（空则进入交互模式）
    Session        string           // 会话标识
    Agent          string           // 指定代理 ID
    Model          string           // 模型覆盖
    Workspace      string           // 工作目录
    Debug          bool             // 调试模式
    Quiet          bool             // 静默模式
    EmbeddedAgents []EmbeddedAgent  // 嵌入式代理列表
    BeforeOutput   func()           // 输出前回调
}
```

### 3. EmbeddedAgent

嵌入式代理定义，允许动态添加自定义代理：

```go
type EmbeddedAgent struct {
    ID          string    // 代理唯一标识
    Name        string    // 显示名称
    Description string    // 描述
    Prompt      string    // 系统提示词
    Soul        string    // 灵魂文件内容（追加到 Prompt）
    Skills      []string  // 启用的技能
    Tools       []string  // 启用的工具
    NoHistory   bool      // 禁用历史记录
}
```

## 主要功能

### 运行代理

```go
// 单次消息模式
err := rt.Run(picoclaw.RunOptions{
    Message: "Hello, world!",
    Model:   "gpt-4",
    Session: "my-session",
})

// 交互模式（Message 为空）
err := rt.Run(picoclaw.RunOptions{
    Model: "gpt-4",
})
```

### DirectRunner

可复用的直接运行器，适合多次调用：

```go
runner, err := rt.NewDirectRunner(picoclaw.RunOptions{
    Model: "gpt-4",
})
defer runner.Close()

// 多次处理
resp1, err := runner.ProcessDirect(picoclaw.RunOptions{Message: "Hello"})
resp2, err := runner.ProcessDirect(picoclaw.RunOptions{Message: "How are you?"})
```

### 图像生成

```go
result, err := rt.GenerateImage(ctx, picoclaw.ImageOptions{
    Prompt: "A beautiful sunset over mountains",
    Model:  "dall-e-3",
    Size:   "1024x1024",
})
```

## 配置解析

### 模型解析优先级

1. `RunOptions.Model` 显式指定
2. Agent 配置中的 `Model.Primary`
3. `cfg.Agents.Defaults.ModelName`
4. `cfg.Agents.Defaults.ModelFallbacks` 列表
5. `cfg.ModelList` 中第一个可用模型

### Agent 解析

1. 查找嵌入式代理列表
2. 查找配置中的 Agent（按 ID 或 Name 匹配）
3. 使用默认 Agent（ID: "main"）

### 工作空间配置

- 自动设置 `RestrictToWorkspace = false`
- 添加工作空间到 `AllowReadPaths` 和 `AllowWritePaths`
- 所有 Agent 的工作空间统一指向配置目录

## 响应处理

### 空响应恢复

当模型返回空响应时，系统会自动重试：

1. 使用 `emptyResponseRetryPrompt` 重试当前会话
2. 如果仍失败，创建新会话重试
3. 最多重试 2 次

### 响应标准化

```go
func normalizeDirectResponse(resp string, err error) (string, error)
```

- 去除首尾空白
- 检测空响应和哨兵值
- 返回标准化后的响应或错误

## 工具支持

嵌入式代理可启用的工具：

| 工具名 | 说明 |
|--------|------|
| `skills` | 技能系统 |
| `find_skills` | 技能搜索 |
| `web` / `web_search` | 网页搜索 |
| `web_fetch` | 网页抓取 |
| `exec` / `bash` / `shell` | 命令执行 |
| `read_file` | 读取文件 |
| `write_file` | 写入文件 |
| `edit_file` | 编辑文件 |
| `append_file` | 追加文件 |
| `list_dir` | 列出目录 |
| `spawn` | 生成进程 |
| `subagent` | 子代理 |

## 依赖

- `github.com/sipeed/picoclaw/pkg/agent` - 代理循环
- `github.com/sipeed/picoclaw/pkg/bus` - 消息总线
- `github.com/sipeed/picoclaw/pkg/config` - 配置管理
- `github.com/sipeed/picoclaw/pkg/logger` - 日志系统
- `github.com/sipeed/picoclaw/pkg/providers` - 模型提供者
- `github.com/sipeed/picoclaw/pkg/skills` - 技能系统
- `github.com/sipeed/picoclaw/pkg/auth` - 认证

## 环境变量

| 变量名 | 说明 |
|--------|------|
| `PICOCLAW_HOME` | picoclaw 主目录 |
| `PICOCLAW_CONFIG` | 配置文件路径 |
| `PICOCLAW_BUILTIN_SKILLS` | 内置技能路径 |

## 使用示例

```go
package main

import (
    "fmt"
    "github.com/sjzsdu/tt/internal/picoclaw"
)

func main() {
    // 加载运行时
    rt, err := picoclaw.Load(picoclaw.Options{
        Home: "/Users/user/.picoclaw",
    })
    if err != nil {
        panic(err)
    }

    // 单次对话
    err = rt.Run(picoclaw.RunOptions{
        Message: "Explain quantum computing in simple terms",
        Model:   "gpt-4",
        Quiet:   true,
    })
    if err != nil {
        panic(err)
    }

    // 使用嵌入式代理
    err = rt.Run(picoclaw.RunOptions{
        Message: "Write a poem about AI",
        EmbeddedAgents: []picoclaw.EmbeddedAgent{
            {
                ID:     "poet",
                Name:   "AI Poet",
                Prompt: "You are a talented poet. Write beautiful verses.",
                Tools:  []string{"write_file"},
            },
        },
        Agent: "poet",
    })
}
```
