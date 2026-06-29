# 多 formula 运行能力的 MVP 方向与阶段边界（tt-4ce）

本文档基于已完成的 research beads，收敛 `tt` 第一阶段“多个 formula 一起运行并统一监视”的 MVP 方向、进入条件、非目标与后续实现切片边界，供后续 implementation beads 直接作为上游决策依据。

## 1. 本 bead 在依赖树中的位置

- 本 bead：`tt-4ce`，目标是把前置研究从“事实集合”收口成**阶段性产品决策**。
- 已完成上游依赖：
  - `tt-f08`：产品形态优先收敛为“多个独立 run + 统一监视视角”；
  - `tt-iqj`：当前真实顶层执行单位仍是**单个 workflow run**，loop 并发不是多 formula 顶层并发；
  - `tt-d9e`：run store / dashboard / web API / 前端状态整体都是 **single-run shaped**，多 run 需要新增聚合层；
  - `tt-q3o`：workspace 与副作用隔离提示第一阶段不能默认允许任意 formula 顶层并发；
  - `tt-ueo`：step 安全矩阵说明不同 step kind 在顶层多 run 下的风险差异很大，需要显式缩范围；
  - `tt-0gu`：已有 loop 并发、schedule、resume/human input、dashboard reopen、worktree 等可复用拼图已被系统盘点。
- 本 bead 的下游阻塞：
  - `tt-2tr`：需要以本文档为依据，定义“什么场景属于第一阶段必须验证”的最小测试矩阵。

因此本 bead**不实现 runtime / dashboard / run store 改动**，只负责输出一个足够具体、可指导实现和验证的 MVP 决策记录。

## 2. 结论先行

### 2.1 推荐的第一阶段 MVP 方向

推荐的 MVP 方向是：

> **支持一次发起多个彼此独立的 formula runs，并提供一个以“聚合监视 + 单 run drill-down”为中心的统一观察入口。**

更具体地说，phase 1 优先做的是：

1. **多个独立 run 的归组与聚合观察**，而不是父子 formula 编排；
2. **统一监视主页 / 聚合列表 / 汇总状态**，而不是把现有单 run dashboard 直接改造成 multi-run 巨型状态对象；
3. **保留现有 run-scoped 交互**（resume、human input、repair、final report chat、run open），聚合页先负责摘要、告警和跳转；
4. **只允许一批“低耦合、可隔离、step 风险可控”的 formula 场景进入第一阶段**，不承诺任意 formula 组合都可安全并发。

### 2.2 不推荐作为第一阶段主方向的方案

以下方向不作为 phase 1 主路线：

- **父 formula 编排多个子 formula**：会过早把问题升级成 runtime/orchestration 语义设计；
- **完整 supervisor / batch execution 平台**：概念和实现都过重，超出第一阶段最小用户价值；
- **任意 formula 顶层并发平台**：与 workspace、副作用、repair、human input 风险研究结论冲突；
- **一次性把所有单 run 交互都抬升到聚合层**：会让 scope 膨胀，削弱可交付性。

## 3. 为什么这个 MVP 最合适

### 3.1 与当前真实系统边界最连续

`tt-iqj` 已经确认：

- 当前顶层执行单位是一个 workflow run；
- `run.Store` / `state.json` / `logs.jsonl` / `patches/<run-id>.json` / human input / repair / dashboard 都围绕**单一 run_id**；
- loop 并发只是单 run 内的局部并发。

因此第一阶段最合理的演进方式，不是重写顶层 runtime，而是：

- 保持多个独立 run 继续复用现有单 run 生命周期；
- 在它们之上增加一层**聚合读模型与聚合观察面**。

这条路线最贴合现有代码形状，也最容易控制实现风险。

### 3.2 与用户价值最直接对齐

`tt-f08` 已经把原始问题定义收敛为两件事：

1. 能不能让多个 formula 一起启动/一起存在；
2. 能不能在 web 中统一看它们的状态、异常、结果和日志入口。

第一阶段若能做到：

- 批量发起多个独立 run；
- 在一个统一入口中看到它们各自的状态、进度、失败、waiting input、完成结果摘要；
- 点进某个 run 继续用原有 dashboard 明细能力；

就已经交付了清晰而直接的用户价值，而不需要先发明复杂的父子 orchestration 语义。

### 3.3 与风险研究结论一致

`tt-q3o` 与 `tt-ueo` 的共同结论是：

- 多个 formula 顶层并发并不天然安全；
- 安全性高度依赖 workspace 隔离、外部副作用、step kind 语义和幂等性；
- human input / repair / final report chat / workspace-mutating script/tool 等能力都不能被简单视作“可直接并发”。

所以 phase 1 必须是**受约束的并发能力**，而不是“任何 formula 都可并发”的开放平台。

## 4. 第一阶段 MVP 的进入条件

以下条件应作为进入 phase 1 的明确前提。

### 4.1 执行模型前提：独立 run，而非父子运行树

phase 1 的每个 formula 仍然以独立 run 执行：

- 独立 `run_id`
- 独立 run store 目录
- 独立 `state.json` / `logs.jsonl` / step artifacts
- 独立 dashboard snapshot
- 独立 resume / human input / repair / final report chat

聚合层只负责：

- 归组；
- 汇总；
- 展示；
- 跳转；
- 可选的有限批量控制（如果后续 bead 证明值得纳入）。

### 4.2 工作负载前提：只纳入低耦合公式组合

第一阶段应优先支持满足以下特征的 formula 组合：

- 彼此**没有跨 formula 数据依赖**；
- 不要求共享同一运行时上下文；
- 可以在用户心智上被理解为“多条并列任务”；
- 即使某个 run 失败，也不需要复杂的级联回滚；
- 可以通过独立 workspace / worktree / invocation cwd 边界减少互相污染。

典型更接近 MVP 的场景：

- 多个独立仓库/目录上的相似巡检型 formula；
- 多个相互独立的报告、检查、只读分析工作流；
- 同一类任务在不同输入对象上的批量执行。

不适合作为第一阶段默认支持的场景：

- 一个 formula 的输出必须驱动另一个 formula 的输入；
- 多个 formula 必须共享同一工作区并同时修改相同文件；
- 需要统一事务、统一取消传播、统一补偿逻辑；
- 需要批次级 repair、批次级 human input broker、批次级 final report synthesis。

### 4.3 安全前提：以“默认保守”替代“默认放行”

基于 `tt-ueo` 的 step 安全矩阵，phase 1 应默认采用保守策略：

- 优先纳入**只读、幂等、可隔离**的 step 组合；
- 对 workspace-mutating、side-effectful、non-idempotent steps 保持显式限制；
- 对需要人工输入、修复回合、长时间交互的 run，只在聚合层暴露摘要与跳转，不默认做批量交互。

也就是说，MVP 的核心不是“尽量多支持”，而是“先保证能安全地支持一小类高价值场景”。

## 5. 第一阶段最小用户价值

MVP 至少应让用户做到以下事情：

1. **一次启动多个独立 formula run**；
2. **在一个统一入口看到这批 runs 的摘要状态**；
3. **快速识别哪些 run 正在运行、已完成、失败、等待人工输入**；
4. **点进某个 run 打开已有单 run dashboard 明细页**；
5. **不丢失现有 run-level 生命周期能力**，包括 resume、input、repair、final report chat 等。

这意味着 phase 1 的用户价值关键词是：

- 批量发起
- 聚合观察
- 异常聚焦
- 明细跳转
- 保持单 run 心智连续

而不是：

- 跨 formula 编排
- 跨 run 数据流
- 统一事务控制
- 任意批量操作平台

## 6. 明确的非目标（本阶段不做）

以下事项应明确排除在 phase 1 之外：

### 6.1 不做父子 formula 编排语义

phase 1 不引入：

- 父 run / 子 run 的强执行语义；
- 跨 formula step 依赖图；
- formula A 完成后自动将输出注入 formula B 的统一 runtime 通道；
- “一个超级 workflow”内嵌多个独立 formula 的完整设计。

### 6.2 不做完整的多 run 交互控制台

phase 1 不承诺：

- 聚合页直接处理所有 human input；
- 聚合页直接发起批量 repair 决策；
- 聚合页提供批量 resume / 批量 retry-step / 批量 stop 的完整交互闭环；
- 聚合页替代单 run dashboard 成为唯一操作面。

第一阶段聚合页以“**摘要 + 告警 + 跳转**”为主。

### 6.3 不承诺任意 formula 组合都可安全并发

phase 1 不承诺：

- 任意 builtin formula 都能直接纳入多 run 并发；
- 任意自定义 formula 都能零配置并发运行；
- 共享同一 workspace 的写操作场景有统一安全保证；
- 复杂外部系统副作用在批量运行时都具备幂等保护。

### 6.4 不在本阶段定稿详细实现设计

本文档不试图定稿：

- 最终 CLI 命名；
- 最终 store schema；
- 最终 dashboard 路由与前端组件树；
- 最终 group 对象是否命名为 batch / run_group / supervisor；
- 完整 roadmap。

这些内容留给后续 implementation / validation beads 细化。

## 7. 对 run store / dashboard / runtime 的影响范围（研究结论级）

本 bead 不展开技术设计，但需要给下游实现 bead 一个明确的影响面地图。

### 7.1 run store：需要“单 run 之上的归组信息”，不是重写单 run 存储

从 `internal/formula/run/store.go` 和 `tt-d9e` 的结论看，phase 1 更像需要补充：

- run 归组元数据；
- group 到 runs 的映射；
- group 级摘要读取入口；
- 可能的聚合事件源或索引。

而不是推翻：

- 现有单 run 目录结构；
- 现有 `run.json` / `state.json` / `logs.jsonl` 模型；
- 现有 `run.Resolve` / `run.List` / `run open` 的单 run 语义。

### 7.2 dashboard / web：需要新增“聚合监视页”，单 run dashboard 保留为 drill-down

从 `tt-d9e` 看，现有 dashboard：

- 服务端 state 是单个 `ui.Snapshot`；
- 前端 hook/state 只消费一个 snapshot；
- stop/input/repair/chat handler 都默认绑定当前 run。

因此 phase 1 更合理的影响面是：

- 新增多 run 聚合数据接口与聚合前端页面；
- 保留现有单 run dashboard 作为明细页；
- 只在必要时补充从聚合页跳转到单 run 页的桥接信息。

### 7.3 runtime：第一阶段尽量避免改写顶层执行器语义

从 `tt-iqj` 可知，当前 runtime 顶层就是一个 workflow executor 对一个 run。

因此 phase 1 应尽量避免：

- 把 executor 改成 batch orchestrator；
- 让一个 snapshot 直接包含多个 workflow 的执行图；
- 在 runtime 内引入完整 supervisor 生命周期。

更合适的方向是：

- 在 runtime 之外增加多次独立 run 的发起/归组能力；
- 由聚合层消费多个独立 run 的已有状态；
- 让 runtime 继续对单个 run 保持稳定抽象。

## 8. 后续 implementation beads 可采用的最小切片

基于本次收口，后续实现 bead 可以优先考虑如下最小切片。

### 切片 A：只读聚合视角

目标：

- 能看到一组 run 的基础摘要；
- 能查看状态、时间、formula 名称、waiting input / failed 标记；
- 能跳转到单 run 详情。

这是最小且风险最低的第一刀，因为它：

- 不改变 run-level 交互语义；
- 能最快兑现“统一监视”的用户价值；
- 与 `tt-d9e` 的聚合读模型建议完全一致。

### 切片 B：批量发起 + 归组元数据

目标：

- 一次创建多个独立 run；
- 为这批 run 建立归组关系；
- 聚合页能基于 group 查看它们。

这是让“统一监视”从静态 run 列表变成“同一次动作产生的一组 runs”的关键切片。

### 切片 C：受限的聚合告警与过滤

目标：

- 仅增加 waiting input / failed / running 等聚合筛选；
- 不直接做批量交互；
- 先帮助用户快速定位异常 run。

这是比“完整控制台”更可控的第二阶段增强。

## 9. 对 tt-2tr 的直接输入

`tt-2tr` 在定义最小验证矩阵时，应默认围绕本文档的 MVP 方向构建场景：

- 多个独立 run，而不是父子 formula；
- 聚合观察 + 单 run drill-down，而不是聚合页承载全部交互；
- 低耦合、可隔离场景优先；
- waiting input / failed / completed / running 的可见性优先；
- workspace 冲突、side effect、non-idempotent steps 应作为“限制条件/负面样例”进入验证，而不是被当作默认支持能力。

## 10. 最终决策摘要

第一阶段 MVP 应明确收敛为：

> **多个独立 formula run 的批量发起与统一监视。**

其关键边界是：

- **做**：归组、聚合摘要、统一观察、异常定位、单 run drill-down；
- **不做**：父子编排、跨 formula 数据流、完整聚合交互控制台、任意 formula 无约束并发；
- **前提**：场景需低耦合、可隔离、step 风险受控；
- **实现策略**：尽量复用现有单 run runtime/store/dashboard，把新增复杂度放在聚合层。

这个决策既能承接前置 research beads 的事实结论，也能为后续验证 bead 和 implementation beads 提供足够清晰的上游依据。
