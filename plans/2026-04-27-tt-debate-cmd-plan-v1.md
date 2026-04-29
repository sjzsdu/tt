# tt debate CMD 实施计划（参考 create-cmd 技能风格）

## Objective

在 `tt` CLI 中新增 `debate` 子命令：用户输入主题后，由预设的两个辩手 Agent 按人设多轮辩论，并由裁判 Agent 协调与裁决，最终输出“你一言我一语”的辩论记录（文本与可选 JSON）。

## Initial Assessment

### Project Structure Summary

- 现有 CLI 使用 Cobra 根命令并统一通过 `Execute()` 启动，适合按同一模式增量新增子命令（来源：`main.go:10-14`，`cmd/root.go:5-18`）。
  - Implication: `debate` 可直接采用与 `agent/markdown/conversation` 一致的命令注册方式。
- `agent` 子命令已完成“CLI 参数 -> 合并配置 -> 加载 Runtime -> 执行”闭环（来源：`cmd/agent.go:49-107`，`internal/picoclaw/runtime.go:31-56`）。
  - Implication: `debate` 不需要重复造模型调用轮子，可复用 Runtime。
- 当前 Runtime 已支持按 agent ID/name 解析与单次 direct 调用（来源：`internal/picoclaw/resolve.go:99-117`，`internal/picoclaw/agent.go:55-66`）。
  - Implication: 可通过“多次单轮调用 + 状态机编排”实现辩论流程。
- 配置系统支持 global/project 合并与命令行覆盖策略（来源：`internal/ttconfig/config.go:63-92`，`internal/ttconfig/config.go:94-140`，`cmd/agent.go:65-83`）。
  - Implication: `debate` 默认人设、裁判、轮数建议接入 `ttconfig`。
- 仓库当前无测试文件（`*_test.go`）可直接复用（来源：项目搜索结果，未命中）。
  - Implication: 需在计划中明确最小验收路径与可重复手工验证脚本。

### Relevant Files Examination

- `cmd/agent.go:22-47`：命令声明 + flags 绑定。
- `cmd/agent.go:56-90`：配置合并与 Runtime 加载。
- `cmd/conversation.go:126-160`：复杂命令的 init + flag 组织风格。
- `internal/picoclaw/agent.go:18-70`：一次请求执行路径（ProcessDirect / ProcessDirectForAgent）。
- `internal/picoclaw/resolve.go:26-69`：模型、agent、session 的解析与校验。
- `internal/ttconfig/config.go:19-49`：配置结构定义入口。
- `internal/ttconfig/config.go:94-140`：配置 merge 规则扩展点。

### Assumptions（含 create-cmd 参考说明）

- `create-cmd` skill 已存在于仓库内的 `.forge/skills/create-cmd/SKILL.md`，且其约束与当前 `tt` 项目的 Cobra 组织方式一致（来源：`.forge/skills/create-cmd/SKILL.md:1-177`）。
- 本计划按该 skill 的约定组织新增命令：
  1) 单文件定义 `var xxxCmd = &cobra.Command{}`；
  2) `init()` 中注册到 `rootCmd` 或父命令；
  3) flags 绑定紧随注册放在同文件；
  4) 非平凡逻辑下沉到显式 helper；
  5) 结束后执行 `gofmt` 与 `go build ./...` 验证。
  （参考：`.forge/skills/create-cmd/SKILL.md:10-16`，`.forge/skills/create-cmd/SKILL.md:20-39`，`.forge/skills/create-cmd/SKILL.md:52-79`，`.forge/skills/create-cmd/SKILL.md:153-160`，以及现有实现 `cmd/agent.go:22-47`、`cmd/conversation.go:126-160`）

## Prioritized Risks and Challenges

1. **多轮状态机与终止条件不明确**（最高优先级）
   - 原因：这是 debate 核心能力，若无清晰终止协议容易死循环或输出混乱。
2. **裁判协调输出不可机器判定**
   - 原因：裁判既要主持又要裁决，若只用自然语言难以稳定驱动流程。
3. **角色人设漂移（串角色）**
   - 原因：多轮上下文叠加容易导致辩手失去立场一致性。
4. **配置/CLI 优先级冲突**
   - 原因：若不沿用既有覆盖逻辑，用户体验会与已有命令不一致。

## Implementation Plan

- [ ] **Task 1（Status: Not Started）创建 `cmd/debate.go` 命令骨架（create-cmd 风格）**：定义 `debateCmd`、`Use/Short/Long/Example`、`RunE`，并在 `init()` 完成注册与 flags 绑定（topic、agents、judge、rounds、output、session/model/debug 覆盖）。
  - Rationale: 先建立稳定 CLI 接口，确保后续引擎与输出对齐入口参数。

- [ ] **Task 2（Status: Not Started）定义 debate 领域数据结构（建议放在 `cmd/debate.go` 或 `internal/picoclaw/debate.go`）**：包括 `DebateRequest`、`DebateTurn`、`JudgeDecision`、`DebateResult`。
  - Rationale: 先有统一数据模型，才能保证文本渲染与 JSON 输出一致。

- [ ] **Task 3（Status: Not Started）实现辩论回合编排器**：按 A→B→Judge 循环执行，维护轮次计数、累计上下文与结束原因（round-limit / judge-stop / error-stop）。
  - Rationale: 将“模型调用”与“流程控制”分离，降低复杂度并提升可维护性。

- [ ] **Task 4（Status: Not Started）实现裁判协调协议与解析器**：约定裁判输出规范（例如 `decision=CONTINUE|STOP`, `focus=...`, `reason=...`），解析失败时回退到安全策略。
  - Rationale: 保障流程可判定、可继续、可结束，避免自然语言歧义卡死。

- [ ] **Task 5（Status: Not Started）接入 Runtime 的多 agent 调用**：复用 `ResolveRunOptions` 和 `ProcessDirectForAgent` 路径，按当前轮次指定 agent 执行。
  - Rationale: 复用现有稳定能力，避免新增 provider 层风险。

- [ ] **Task 6（Status: Not Started）扩展 `ttconfig` 增加 DebateConfig**：新增默认辩手、默认裁判、默认轮数、默认输出格式，并扩展 `Merge`。
  - Rationale: 与 `agent/markdown/conversation` 一致，支持全局与项目级策略。

- [ ] **Task 7（Status: Not Started）实现输出渲染器**：
  - [ ] 文本模式：严格“你一言我一语”顺序排版，包含轮次标记与最终裁决。
  - [ ] JSON 模式：输出完整 turns、judge decisions、summary、metadata。
  - Rationale: 同时满足人读与程序消费场景。

- [ ] **Task 8（Status: Not Started）完善错误与降级机制**：覆盖 agent 不存在、模型不可用、单轮调用失败重试（有限次）与最终兜底总结。
  - Rationale: 多轮调用失败概率更高，必须保证命令可预期退出。

- [ ] **Task 9（Status: Not Started）定义并执行最小验收清单（手工/脚本化）**：验证默认参数、自定义参数、裁判提前结束、JSON 结构稳定。
  - Rationale: 当前缺少现成测试基线，需通过场景验收保障质量。

## Verification Criteria

- [ ] 输入主题后，命令能生成至少 1 轮完整链路（A 发言、B 发言、Judge 决策）。
- [ ] 当 Judge 发出 STOP 时，流程能提前终止并输出终止原因与最终裁决。
- [ ] `--output text` 与 `--output json` 都可稳定输出；JSON 字段固定、可解析。
- [ ] 指定不存在 agent/model 时，命令返回明确错误且不进入不完整回合。
- [ ] CLI 参数优先级高于 project/global 配置，行为与 `agent` 命令一致。

## Potential Risks and Mitigations

1. **裁判输出格式不稳定导致解析失败**  
   Mitigation: 使用强约束模板 + 解析失败回退（默认 CONTINUE 到轮数上限或安全 STOP）。

2. **多轮上下文膨胀导致延迟与成本上升**  
   Mitigation: 设置默认最大轮数并支持裁判提前结束；上下文仅保留必要摘要与最近回合。

3. **辩手立场漂移影响辩论质量**  
   Mitigation: 每轮注入“角色立场 + 禁止越位 + 当前待回应点”的系统化提示词片段。

4. **配置扩展破坏兼容性**  
   Mitigation: DebateConfig 字段全部可选，保留旧配置零改动可运行。

## Alternative Approaches

1. **Approach A（推荐）**：在 `cmd/debate.go` 直接实现编排 + 调用 Runtime。  
   Trade-off: 落地快、改动小；但后续若有多入口复用，需再拆分引擎层。

2. **Approach B**：新增 `internal/picoclaw/debate_engine.go` 承载状态机，`cmd/debate.go` 仅做参数与输出。  
   Trade-off: 架构更清晰、复用性更好；初次实现成本更高。
