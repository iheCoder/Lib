---
name: shadow-claw
description: |
  Shadow Claw Investigation Engine (影爪调查引擎).
  面向复杂系统故障的自主调查循环。当用户报告故障、异常、性能问题，或需要进行根因分析时激活。
  不是一次性问答，而是驱动 观测->假设->验证->扩展世界 的闭环调查，直到找到真正的根因或明确的问题方向。
  协调 evidence-builder 和 hypothesis-validator 两个子 skill 完成调查。
---

# Shadow Claw — 影爪调查引擎

> **不是"帮你看看日志"，而是像高级工程师一样驱动一轮完整的事故调查。**

## 核心哲学

Shadow Claw 的价值不是猜得准，而是 **知道自己哪里不确定，并主动去消除不确定性**。

> 没有 Validator，系统会在一个错误世界里越分析越自信。
> 有 Validator，它会不断跳出错误世界。

## 定位

Shadow Claw 是整个调查系统的 **编排器和循环驱动器**。

它不亲自做事实提取（那是 Evidence Builder 的事），
也不亲自做假设审判（那是 Hypothesis Validator 的事），
它做的是 **驱动调查循环，直到收敛**。

### 三个角色，一个循环

```
  Evidence Builder  -->  Hypothesis Engine  -->  Hypothesis Validator
      (事实层)              (猜测层)                 (审判层)
        ^                                              |
        |          解释不成立 -> 扩展世界                |
        +----------------------------------------------+
```

| 角色 | 决定什么 |
|------|---------|
| Evidence Builder | 你 **看到了什么** |
| Hypothesis Engine | 你 **猜是什么** |
| Hypothesis Validator | 你 **该去看什么** |

> **Evidence Builder 不会主动扩展世界。是 Hypothesis Validator 在"逼它去扩展世界"。**

### Capability Pool（能力池）

进入不同"世界"需要不同的工具。大部分工具是 Agent 天然具备的（`grep`、`git`、`kubectl`），
但有些需要专门的 skill 才能进入：

| 外部能力 | 能进入的世界 | 说明 |
|---------|------------|------|
| aliyun-sls-trace | trace, log | 阿里云链路追踪与日志查询 |
| mysql-readonly-query | database | MySQL 只读查询 |
| web-access | external_knowledge | 外部信息获取与网页浏览 |

其他世界（code / deploy / scaling / config / network / metric / environment）用基本操作就能进入。

详见 [references/capability-pool.md](references/capability-pool.md)。

## 何时使用

- 用户报告系统故障、性能异常、请求超时等
- 需要从症状出发，找到根因或至少确定问题方向
- 问题复杂到不能一眼看出答案（多源、跨层、表象模糊）
- 需要排除多个可能性，而不是凭直觉给结论

---

## 调查循环：完整流程

### 总览

```
输入(症状) -> 循环 { 采集 -> 假设 -> 验证 -> [扩展世界] } -> 报告 -> 处置
```

最多执行 **5 轮**循环（可配置），每轮世界都会变大，认知都会升级。

调查过程中 **随时可以询问用户**：用户可能知道"最近改过什么"、"这个问题以前出现过吗"等关键信息。不要闭门造车。

---

### Phase 0: 接收症状 (Intake)

**输入**: 用户的工单、告警、口头描述、截图等任何形式的症状。

**执行**:

1. 从症状中提取: 实体、时间窗、环境、症状描述
2. **向用户确认**：提取是否准确？有没有遗漏的上下文？
3. 检查 project-cognition 中是否有相关的历史经验

---

### Phase 1: 初始观测 (Observe)

**调用 Evidence Builder** 进行初始采集。

第一轮采集所有直接可见的数据源（log / trace / metric / ticket）。
如果需要进入 trace 世界，使用 aliyun-sls-trace skill；如果需要查数据库，使用 mysql-readonly-query。
其他执行能力池发现。

**关键**: 不仅采集"有什么"，还要主动检查"没有什么"（缺失事实）。
不可达的世界直接记录为缺失事实，影响后续的置信度上限。

---

### Phase 2: 假设生成 (Hypothesize)

**这一步由 Shadow Claw 自己完成（LLM 推理）。**

基于当前证据包，生成候选假设。

**规则**:

- 不能只有一个假设 — 至少要有竞争假设来保持诚实
- 假设应覆盖不同层级的可能性（不要全猜在同一层）
- 每个假设必须能被证伪（否则不是假设，是废话）
- 从经验/直觉出发的强假设也可以，不必刻意凑数量

层级覆盖参考:

| 层级 | 典型假设方向 |
|------|------------|
| 代码层 | 代码变更/SQL退化/逻辑错误 |
| 配置层 | 配置切换/限流变更/缓存开关 |
| 部署层 | 发版/扩缩容/灰度 |
| 依赖层 | 上游抖动/DB过载/缓存击穿 |
| 基础设施层 | 网络/DNS/资源竞争 |
| 观测层 | 时钟偏移/trace失真/日志丢失 |

---

### Phase 3: 假设验证 (Validate)

**调用 Hypothesis Validator** 对每个假设进行四层校验（时间 → 量级 → 范围 → 反事实）。

可以用脚本做规则+统计校验:

```bash
python3 hypothesis_validator/scripts/validate_hypothesis.py --input evidence.json --pretty
```

然后由 LLM 补充反事实校验（第 4 层）。

---

### Phase 4: 决策 (Decide)

根据验证结果，做出下一步决策:

- **有假设被 promote** → 收敛，进入 Phase 5
- **证据不足 (insufficient_evidence)** → 扩展世界，回到 Phase 1
- **有 retain 但无法区分** → 扩展世界获取更多证据
- **全部 reject** → 生成新假设或带不确定性收敛
- **到达最大轮次** → 带不确定性收敛

#### 扩展世界

收集所有假设的 `next_worlds_to_query`，按**判别性信息增益**排序（见 investigation-loop-protocol.md Section 2.2），
选择最能区分竞争假设的世界进行扩展，通过能力池（或天然能力）进入对应世界采集新证据。
新证据增量追加到 Evidence Pack，然后重新生成/验证假设。

**如果某个世界进不去**（缺少工具/权限），记录为缺失事实，不要假装它不存在。

---

### Phase 5: 收敛与报告 (Converge)

调查收敛后，输出调查报告。**报告是给人看的，用 markdown 格式**。

报告结构:

#### 1. 一句话结论

> DB CPU 爆满的主因是发版扩容后 8 个 Pod 同时启动 full-sync-task，负载成倍放大。

#### 2. 关键证据链

按时间排列的因果链:

- **10:20** — 开始发版 v2.3.1→v2.3.2
- **10:22** — Pod 从 1 扩到 8
- **10:23** — 8 个 Pod 同时启动 full-sync-task
- **10:24** — DB CPU 从 50% 阶跃到 100%

#### 3. 被推翻的假设

- ~~代码小问题导致 CPU 升高~~ — 量级不足（预期 +5%，实际 +50%），范围不匹配（L1 vs L4）

#### 4. 剩余不确定性

- 代码变更是否有微弱叠加影响，尚未完全排除

#### 5. 调查过程摘要

每轮做了什么、看了什么、推翻了什么。

---

### Phase 6: 处置 (Resolve)

**找到问题不等于解决问题。这一步把诊断结论转化为行动。**

#### 处置方案格式（markdown，给人看的）

```markdown
## 处置方案

### 🔴 立即止血（现在就做）

1. **限制 full-sync-task 并发数为 1**
   - 原因: 立即降低 DB 负载
   - 风险: 低 — 只是限制并发，不影响功能
   - → 可以直接执行

### 🟡 短期修复（今天内）

2. **为 full-sync-task 增加分布式锁**
   - 原因: 确保全局只有 1 个实例运行全量任务
   - 建议: 在 master 分支开 fix 分支，实现后提 PR

### 🔵 长期预防（本周）

3. **改造发版扩容流程**
   - 原因: 新 Pod 启动时不应自动触发全量任务
   - 建议: 改为延迟启动或主动触发

### 📊 持续监控

4. **为 full-sync-task 并发数添加告警**
   - 阈值: 并发 > 2 时告警
```

#### 处置的执行原则

不是"永远不执行"，也不是"什么都自动执行"。原则是 **按副作用分级**:

| 动作类型 | 副作用 | 处理方式 |
|---------|--------|---------|
| 只读查询、分析、生成报告 | 无 | 直接执行 |
| 创建 fix 分支、写代码、提交到新分支 | 无（不影响主线） | 直接执行 |
| 修改配置文件（本地/非生产） | 低 | 直接执行 |
| 扩缩容、限流调整 | 中 | **征询用户后执行** |
| 回滚版本、重启服务 | 高 | **征询用户后执行** |
| 修改生产数据库/配置 | 高危 | **只给方案，用户自行执行** |

核心:
- **无副作用或可逆操作 → 直接做**（如开 fix 分支修复代码并提交）
- **有副作用但可控 → 征询用户确认后执行**
- **高危不可逆 → 只建议，不碰**

---

## 调查中的用户交互

调查不是闭门造车。以下时机 **应该主动与用户交互**:

| 时机 | 问什么 |
|------|--------|
| Phase 0 (Intake) | "我理解的对吗？还有什么上下文？" |
| 假设生成后 | "你觉得还有可能是什么方向？" |
| 证据不足时 | "你有某某系统的权限/数据吗？" |
| 两个假设无法区分时 | "根据你的经验，更可能是哪个？" |
| 处置方案生成后 | "这个方案可以吗？需要调整吗？" |
| 需要执行有副作用的操作时 | "确认要执行吗？" |

**但不要问无意义的问题**。如果你能自己查到，就不要问用户。

---

## 经验沉淀

调查结束后，如果发现了有价值的模式，应沉淀到 project-cognition:

- **新的排障模式** → `project_cognition/runbooks/`
- **新的系统知识** → `project_cognition/domain-knowledge/`
- **新的原则/禁区** → `project_cognition/core-manifest/`

例如: "full-sync-task 在扩容时会并发启动" 这个知识，值得写入 domain-knowledge，
下次再遇到类似问题时可以更快定位。

---

## 安全约束

### 不做

1. **不在证据不足时给唯一结论** — 宁可说"我还不确定"
2. **不跳过假设直接给答案** — 必须有竞争假设
3. **不忽略被推翻的假设** — 推翻理由是调查严谨性的证明
4. **不无限循环** — 最多 5 轮
5. **不忽略 insufficient_evidence** — 必须去扩展世界
6. **不在用户不知情的情况下执行有副作用的操作**

### 始终做到

1. **每轮都记录做了什么**
2. **缺失事实必须显式标注**
3. **报告中包含推翻过程**
4. **报告中包含剩余不确定性**
5. **处置方案必须可操作**
6. **有价值的经验要沉淀**

---

## 与子 Skill 的关系

```
shadow-claw (编排器)
  |
  +-- evidence-builder (事实层)
  |     +-- 初始采集: Phase 1
  |     +-- 扩展采集: Phase 4 (由 Validator 触发)
  |     +-- 使用能力池中的外部能力 (aliyun-sls-trace 等)
  |     +-- 使用天然能力 (grep, git, kubectl 等)
  |
  +-- hypothesis-validator (审判层)
  |     +-- 四层校验: Phase 3
  |     +-- 输出 next_worlds_to_query -> 驱动世界扩展
  |
  +-- project-cognition (经验层)
  |     +-- 调查前: 读取历史经验
  |     +-- 调查后: 沉淀新发现
  |
  +-- 用户 (人类层)
        +-- 提供上下文、经验判断
        +-- 确认处置方案
        +-- 授权有副作用的操作
```

---

## 参考文档

- Evidence Builder: [../evidence_builder/SKILL.md](../evidence_builder/SKILL.md)
- Hypothesis Validator: [../hypothesis_validator/SKILL.md](../hypothesis_validator/SKILL.md)
- 能力池: [references/capability-pool.md](references/capability-pool.md)
- 调查循环协议: [references/investigation-loop-protocol.md](references/investigation-loop-protocol.md)
- 系统设计理念: [ref_im.md](ref_im.md)
