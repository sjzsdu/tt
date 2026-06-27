# 多 formula 运行产品形态调研（tt-f08）

## 一句话结论

第一阶段应优先研究**批量启动多个独立 run，并提供统一监视视角**，而不是优先做“父 formula 编排多个子 formula”或新增重量级 supervisor/batch 执行模型。

## 背景与目标

用户当前想解决的不是“任意工作流编排平台”问题，而是更具体的两个维度：

1. **执行层**：有时希望多个 formula 可以一起启动或并发存在；
2. **展示层**：希望在 web 中集中观察这些 formula 的状态、进度、结果、异常和日志入口。

本 bead 的目标是先收敛第一阶段产品问题定义，为后续 runtime、observability、workspace、安全矩阵和 MVP bead 提供共同前提。

## 当前产品事实基线

基于现有代码和文档，可以先确认几件事：

- `tt formula run <name>` 的默认运行单位是**单个 formula run**；每次执行都会创建一个独立 run 目录，并可选附带一个 dashboard。
- `tt formula runs` 可以**列出多个历史 run**，但只是 CLI 列表，不是聚合监控页面。
- `tt formula run open <id>` 只能**打开某一个 run** 的 dashboard。
- `tt formula schedule ...` 可以**重复触发同一个 formula 的多次运行**，但每次仍是独立 run，默认也不开 live dashboard。
- 当前 dashboard 前端 `web/apps/formula/src/components/App.tsx` 明显围绕**一个 snapshot / 一个 run** 组织：单 run 的 header、步骤、repair、workspace、human input、final report。

因此，tt 当前已经具备“多个 run 可以存在”的基础，但产品心智与可视化能力仍然是**单 run 中心**。

## 候选产品形态对比

### 方案 A：批量启动多个独立 run + 聚合监视

**定义**

一次用户动作发起多个彼此独立的 formula run；系统为每个 run 维持现有单 run 生命周期，同时新增一个更高层的“聚合观察面”。

**目标用户动作**

- 我想一次启动多个 formula；
- 我想看到它们都在不在跑、谁完成了、谁失败了；
- 我想点进去查看某个具体 run 的详细步骤和日志。

**对用户可见的收益**

- 最贴近“多个 formula 一起运行”这句原始需求；
- 最大化复用现有 `run` / `runs` / `run open` / dashboard；
- 用户仍可保留“每个 run 都能单独打开、恢复、输入、查看日志”的既有习惯。

**与当前单 run 心智模型的差异**

- 单 run 仍然存在，但上面多了一层“run 集合 / 批次 / group”视角；
- 关注点从“这个 run 发生了什么”扩展为“这一组 run 分别发生了什么”。

**最适合作为第一阶段的原因**

- 产品增量最小：不必先发明子 run 语义；
- 技术边界最清晰：先新增聚合观察面，再讨论是否需要更强 orchestration；
- 直接命中用户提到的 web 统一监视诉求。

**主要不足**

- 不能天然表达父子依赖、跨 formula 数据流、集中取消传播等更强编排语义；
- 如果后续需要真正的 orchestrator，第一阶段模型可能还要升级。

### 方案 B：父 formula 编排多个子 formula

**定义**

把“多个 formula 一起运行”建模成一个更大的父级 formula，由它调度和等待多个子 formula。

**目标用户动作**

- 我把多个 formula 当作一个更大的业务流程；
- 我希望它们之间可以有依赖、顺序、并发、汇总结果。

**对用户可见的收益**

- 语义统一：所有事情仍然发生在 formula 世界里；
- 如果做好，未来可以表达更复杂编排。

**与当前单 run 心智模型的差异**

- 不再只是“多个独立 run”，而是“一个大 run 里含多个子 run / 子流程”；
- dashboard、run store、resume、human input 的归属关系会变复杂。

**为什么不适合作为第一阶段优先项**

- 它提前把问题升级成 orchestration 设计问题；
- 当前系统大量能力是 run-scoped，直接进入父子编排会让 runtime、store、dashboard 同时变复杂；
- 容易偏离用户最直接的诉求“我想统一看多个 formula”。

### 方案 C：新增 supervisor / batch 视图与执行模型

**定义**

引入新的上层实体，例如 batch、run group 或 supervisor，对多个 formula run 的创建、状态聚合、取消、观察进行统一管理。

**目标用户动作**

- 我想像看任务批次一样看这一组 formula；
- 我想有一个明确的上层对象来追踪这次多运行任务。

**对用户可见的收益**

- 从概念上最完整；
- 未来最容易承载聚合状态、批量操作、全局取消、汇总结果。

**与当前单 run 心智模型的差异**

- 新增了一层一等模型；
- 用户不再只认识 run，还要认识 batch / group / supervisor。

**为什么不适合作为本 bead 的第一阶段结论**

- 它更像后续 architecture/data model 讨论结果，而不是本轮问题定义的最小起点；
- 一开始就引入 supervisor 容易把研究方向推向“重平台化”。

## 推荐结论

### 推荐的第一阶段问题定义

优先研究：**批量启动多个独立 run，并在 web 中提供统一监视视角**。

换句话说，第一阶段的核心问题不是“如何把多个 formula 融成一个超级 workflow”，而是：

- 如何让多个现有 run 以低耦合方式一起被发起或归组；
- 如何在不破坏单 run 模型的前提下，提供一个高于单 run 的统一观察面。

### 为什么优先这一形态

1. **最贴近原始需求表述**：用户直接说的是“多个 formula 一起运行”和“统一监视”，不是“定义父子 workflow 语义”。
2. **与当前产品模型最连续**：`run`、`runs`、`run open`、schedule 都已存在，可作为研究和演进基础。
3. **能把执行问题和展示问题适度解耦**：先确认 run 的集合视角，再决定是否需要更重的 orchestration 模型。
4. **更利于缩 MVP**：可以先支持相对独立、低耦合、可隔离的 formula 场景，而不是要求所有 formula 之间都能安全协作。

## 第一阶段统一监视的最小信息集合

如果采用“多个独立 run 的聚合监视”形态，统一监视至少需要覆盖：

- **run 标识**：run id、formula 名称、可能的 group/batch 标识；
- **当前状态**：queued / running / waiting_input / completed / failed / stopped；
- **进度摘要**：总步数、已完成数、当前运行步骤、是否有 repair；
- **结果摘要**：最终成功/失败、最终输出是否可查看；
- **异常摘要**：错误消息、失败步骤、是否等待人工输入；
- **日志入口**：能够快速跳转到单 run 的详细日志/步骤页面；
- **时间信息**：开始时间、结束时间、持续时长；
- **workspace 摘要**：必要时展示工作区/隔离方式，便于识别并发副作用风险。

这里的重点是：**聚合视图先做“观察与跳转”，不要求第一阶段直接承载所有单 run 交互细节**。

## 明确不优先的方向

本轮问题定义明确不优先以下方向：

- 分布式调度系统；
- 任意任务队列平台；
- 跨机器执行；
- 完整的父子 workflow 编排语义；
- 具体 CLI/API 命名定稿；
- 最终数据库 / store schema 定稿；
- 前端交互稿与实现细节。

## 对下游 beads 的输入价值

本结论为后续 beads 提供了明确前提：

- `tt-iqj` 应围绕“单 run 真实运行边界”和“哪些能力能支持独立多 run”展开；
- `tt-d9e` 应重点研究“如何从单 run dashboard/store 演进到聚合监视”；
- `tt-4ce` 的 MVP 收敛应默认先考虑“独立 run 聚合监视”，而不是重 orchestration。

## 开放问题

虽然本 bead 已收敛第一阶段问题定义，但仍有几个问题留给后续研究：

1. 这些 run 是由一次 CLI 批量发起，还是由后续某个更高层入口发起？
2. 聚合视图是否需要一等持久化对象（run group / batch），还是先用弱归组也够？
3. human input、repair、stop request 在聚合视图里应只做摘要，还是要支持批量交互？
4. 哪些 formula 类型可以纳入第一阶段 MVP，哪些因 workspace/side effect 风险需要排除？
