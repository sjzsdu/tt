# tt 项目导读

> 最后更新：2026-06-02

`tt` 是一个以 **Cobra 命令集合** 为外壳、以 **Picoclaw 运行时复用**、**Formula 工作流执行** 和 **本地开发自动化** 为核心的 Go CLI 工具箱。它一边提供面向日常文件操作的轻量命令，一边把更复杂的 agent 执行、formula 工作流、技能生成、代码分析等能力收拢到同一个可执行文件里。

如果只看一句话，可以把它理解为：

> 一个把“本地浏览/编辑 + Agent 运行 + 工作流编排 + 技能生成 + 代码理解文档生成”揉进同一套 CLI 体验中的开发辅助平台。

---

## 建议先看什么

### 如果你是第一次接触这个仓库
1. 先看 [项目概览](./overview.md)
2. 再看 [架构与运行模型](./architecture.md)
3. 然后按兴趣深入：
   - 想理解命令分布：看 [命令与模块地图](./module-map.md)
   - 想理解 formula：看 [Formula 工作流系统](./formula-system.md) + [Step Kinds 参考](./step-kinds-reference.md)
   - 想看仓库自带的 formula 模板：看 [内置 Formula 与 Atomic 清单](./builtin-formulas.md)
   - 想理解 agent runtime 复用：看 [Picoclaw 集成与嵌入式 Agent](./picoclaw-integration.md)

### 如果你准备修改命令实现
1. [命令与模块地图](./module-map.md)
2. [架构与运行模型](./architecture.md)
3. 结合 `cmd/` 与对应 `internal/` 包阅读源码

### 如果你准备扩展 formula / workflow 能力
1. [Formula 工作流系统](./formula-system.md)
2. [Step Kinds 参考](./step-kinds-reference.md)
3. [架构与运行模型](./architecture.md)
4. 然后重点看 `cmd/formula/`、`internal/formula/`、`internal/formula/runtime/`、`internal/formula/steps/`、`internal/formularun/`

### 如果你准备接入新的嵌入式 agent 或复用 Picoclaw
1. [Picoclaw 集成与嵌入式 Agent](./picoclaw-integration.md)
2. [命令与模块地图](./module-map.md)
3. 然后看 `internal/agents/` 与 `internal/picoclaw/`

### 如果你准备增强 docs / 代码理解生成能力
1. [项目概览](./overview.md)
2. [命令与模块地图](./module-map.md)
3. 再看 `cmd/docs.go` 与 `internal/agents/embedded/docs-analyst.md`

---

## 这套文档解决什么问题

原始 README 已经覆盖了“有哪些命令、怎么安装、怎么运行”，但这个项目真正难理解的部分不在命令列表，而在下面几个交叉点：

- `tt` 不是单一用途 CLI，而是一个持续扩展的命令容器。
- 一部分命令只是本地 Web UI；另一部分命令会进入 LLM / agent 运行时。
- `formula` 子系统已经接近一个小型工作流引擎，不再只是“模板渲染”。
- `agent`、`debate`、`translate`、`repo2skill`、`nvwa`、`docs` 等命令共享 Picoclaw 运行时，但共享方式是“嵌入式适配”，不是简单 shell 调外部程序。
- `docs analyze` 说明这个仓库已经开始把“代码理解”本身产品化成一个命令。

因此，这套文档更关注：

- 项目到底由哪些子系统组成
- 命令层与内部包如何分工
- formula 是如何从模板编译成可执行图，再被执行和持久化的
- Picoclaw 是如何被加载、裁剪、复用和注入嵌入式 agent 的
- 哪些目录最值得先读，哪些点最适合扩展

---

## 文档结构

```text
ai-docs/
├── README.md                      # 导读与阅读路径
├── overview.md                    # 项目概览、定位、能力边界
├── architecture.md                # 总体架构、分层与关键运行链路
├── module-map.md                  # cmd/ 与 internal/ 的职责地图
├── formula-system.md              # formula 编译、执行、运行记录、控制流
├── step-kinds-reference.md        # step kind 速查与示例
├── builtin-formulas.md            # 内置 formulas / atomics 清单与用途
└── picoclaw-integration.md        # Picoclaw 运行时复用、嵌入式 agent、相关命令
```

---

## 推荐阅读路径

### 路径 A：先建立全局地图
适合第一次接触代码的人：

```text
README → overview → architecture → module-map
```

目标是先搞清楚：
- 项目到底是不是单一工具
- 哪几类命令最重要
- 关键基础设施在哪些目录里

### 路径 B：围绕 formula 深入
适合要改 workflow / 自动化执行的人：

```text
overview → architecture → formula-system → step-kinds-reference → builtin-formulas
```

重点关注：
- formula 如何解析与编译
- Workflow 与 Step 的边界是什么
- typed runtime 如何做图执行、运行时条件判断、loop、resume/retry、script repair、validation advice 与状态落盘
- run store 如何支撑 resume 和 dashboard
- 仓库自带哪些 formula 模板和原子步骤可作为新公式起点

### 路径 C：围绕 agent runtime 深入
适合要改 LLM / agent 相关命令的人：

```text
overview → architecture → picoclaw-integration → module-map
```

重点关注：
- runtime 如何加载 Picoclaw 配置
- embedded agent 如何注入
- 为什么多个命令可以共享同一套 direct runner 模式

---

## 阅读提示

这个仓库最适合按“两层视角”理解：

1. **产品层**：`tt` 对外提供了哪些能力，为什么这些能力会出现在同一个 CLI 里。
2. **运行层**：同一个命令入口下，哪些命令只是本地 HTTP 服务，哪些命令会编译工作流，哪些命令会进入 Picoclaw 代理执行链。

如果能先建立这两层心智模型，再回头看代码，理解成本会明显下降。

---

## 本次文档更新范围

本次不是从零创建，而是在现有 `ai-docs/` 基础上按当前代码进行增量更新。重点调整包括：

- 在概览、架构、模块地图、formula、picoclaw 五篇核心文档中同步新增能力：
  - `tt agent optimize` / `tt agent info`
  - 嵌入式 `agent-optimizer` / `code-research` / `tech-blog-writer` / `writer` / `reporter` 等 agent
  - formula 的 preflight、worktree workspace、environment context、output validation advice、script repair
  - 新的 step kind：aggregate / tool / write_files / retry / loop（parallel + max_concurrency）
  - `internal/formula/ast` / `ir` / `compile` / `builtin` 子包
  - `internal/agentopt` / `internal/formulaui` / `internal/formuladoc` / `internal/formularunview`
  - 前端构建嵌入路径与 `make web-build`
- 新增 [Step Kinds 参考](./step-kinds-reference.md)，集中解释每种 step 的字段、典型 TOML 与使用陷阱
- 新增 [内置 Formula 与 Atomic 清单](./builtin-formulas.md)，列出仓库自带的 formulas 与 atomics
- README 维持导读页定位，加入新文档索引
