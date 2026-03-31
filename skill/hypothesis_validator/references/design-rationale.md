> **怎样让系统不被一个“看起来像答案”的解释骗住。**

这正是 Hypothesis Validator 的意义。

我先给结论：

> **Hypothesis Validator 不是“再猜一次根因”。**
> 它的职责是：**审判当前假设是否站得住。**
> 具体要审三件事：
> 1）时间上对不对，
> 2）量级上够不够，
> 3）范围上像不像。

近两年的 RCA/LLM 方向，基本都在往这个思路靠：先收集和组织诊断信息，再做根因判断；并且越来越重视时空关系、跨源关联和因果回溯，而不是直接让 LLM 对着原始材料“自由发挥”。RCACopilot先做诊断信息收集与归纳；SynergyRCA显式建模时空关系和实体连接；AgentTrace则直接从执行日志重建因果图、再从错误向后回溯根因。OpenTelemetry 官方也把 logs、traces、metrics 定义为可关联的不同 telemetry signals，而不是互不相干的材料堆。([ACM Digital Library][1])

所以，**Hypothesis Validator 的真正定位**应该是：

> **一个“因果合理性审查器”**
> 它不负责发现所有答案，
> 它负责淘汰那些“听起来像、但实际上解释不了现象”的答案。

---

## 一、它究竟该做什么？

我把它压成一句人话：

> **凡是一个假设解释不了“为什么是现在、为什么这么大、为什么这么广”，它就不配当主因。**

这三问就是 Validator 的核心。

### 第一问：为什么是现在？

这是时间问题。

如果数据库 CPU 在 10:32 突然从 50% 拉到 100%，而 10:20 刚发过版，那么“发版”至少要进入候选因果集合。
如果一个假设完全解释不了“为什么在这个时间点突变”，它就不完整。Google 的 SRE postmortem 实践也把“时间线重建”和事实化叙述放在核心位置，因为根因分析不是列现象，而是解释事件如何按时间发生。([sre.google][2])

### 第二问：为什么这么大？

这是量级问题，也是你刚刚抓得最准的点。

如果某段代码的小低效理论上只能让 CPU 多 3% 到 5%，而系统观测到的是 50% 到 100% 的阶跃变化，那这个假设最多只能当“次要因子”，不能当主因。
这和很多 RCA 研究从“相关性”往“因果性”走是同一路线：不能因为某个因素存在，就把它当决定性原因；要看它是否足以解释观测到的影响。RC-LLM 把 RCA 明确建模成“深时序因果推理”；更广义的因果/反事实方向也强调，要区分“能共存的解释”和“足以产生结果的解释”。([arXiv][3])

### 第三问：为什么这么广？

这是范围问题。

是单机、单 pod、单请求，还是整个 fleet、整批 pod、整段时间窗都同步异常？
如果是 8 个 pod 一起异常，那“某个请求的特殊 SQL”这类局部解释，优先级就应该下降；而“发版/扩容/全量任务/配置切换”这类系统性解释，优先级应该上升。SynergyRCA 之所以要建 StateGraph 和 MetaGraph，本质就是为了把“时间关系 + 空间范围 + 实体连接”一起拿来判断，而不是只盯单点日志。([arXiv][4])

---

## 二、Hypothesis Validator 不该做什么？

这点也很重要。

它**不应该**：

* 再重新读一遍所有材料自由发挥
* 直接代替 LLM 生成漂亮报告
* 对每个假设做玄学打分
* 一上来就试图证明谁是根因

它应该做的是更冷酷的事：

> **先证明哪些假设不够格。**

这和科学推理、事故调查都更接近：不是先找一个你喜欢的答案，而是先排除不合理解释。

---

## 三、它该怎么做？我建议做成“四层校验器”

### 第 1 层：时间校验（Temporal Validation）

这是最先做的，因为最硬。

它检查几类事：

* 候选原因是否发生在结果之前
* 候选原因与异常开始时间的间隔是否合理
* 是否存在明显的 change point（突变点）
* 候选原因和异常的时间窗口是否重叠或邻近

在工程上，它不一定非要复杂算法。第一版可以很朴素：

* 记录关键事件时间：发版、扩容、配置变更、任务启动、告警触发
* 记录指标变化时间：CPU、QPS、错误率、RT、DB CPU
* 做简单窗口比对：`cause_time <= effect_time` 且 `effect_time - cause_time < max_reasonable_lag`

如果后面要升级，再加 change point detection，用来自动识别“从什么时候开始不一样了”。RCA 领域和更广泛的时序分析里，change point detection 一直被当成关键基础能力，因为它能把“平稳波动”与“结构性变化”区分开。([ScienceDirect][5])

### 第 2 层：量级校验（Magnitude Validation）

这是你最关心、也最容易把系统从“会说话”拉到“像工程师”的层。

它做的不是精确建模，而是**量纲 sanity check**：

* 这个假设理论上最多能带来多大影响？
* 观测到的影响有多大？
* 两者是否在一个数量级上？

比如你的例子里：

* “代码里有一点低效”
  预期影响：CPU +5% 左右
* 实际观测：DB CPU 从 50% 到 100%

那这个假设就应该被打上：

> **量级不足，不足以解释主异常**

这里不需要完美公式，第一版可以直接做“经验上界”：

* 单次 SQL 变差：通常导致单请求成本上升，但不会凭空让 fleet-wide CPU 翻倍
* pod 数量从 1 → 8，且每个 pod 触发全量任务：这非常容易解释整体负载乘法放大

也就是说，Validator 不是要精确模拟世界，而是先做**数量级否决**。这和因果/反事实路线里“不能只看相关性，要看干预后能否产生足够大的效果”是同一哲学。([arXiv][6])

### 第 3 层：范围校验（Scope Validation）

它要判断：

* 假设解释的是局部异常，还是全局异常？
* 观测现象是局部的，还是 fleet-wide 的？
* 这个假设能否覆盖问题的空间分布？

例如：

* 单个请求代码 bug
  更像局部
* 发版扩成 8 pod + 全量任务
  更像全局同步

如果“现象很广，解释很窄”，就要扣分。

### 第 4 层：反事实校验（Counterfactual Validation）

这是最强的一层，也最像高手。

它问的不是“这个假设能不能解释”，而是：

> **如果这个假设是真的，我们还应该看到什么？**

比如：

如果真是“代码小问题导致 DB CPU 爆掉”，通常还应该看到：

* 某些特定 SQL 的耗时显著拉长
* 某类请求量或某个路径占比异常高
* 影响可能偏局部，而不是和 pod 数线性同步

如果真是“8 个 pod 同时跑全量任务”，通常还应该看到：

* 发版/扩容时间与 CPU 抬升接近
* job/cron/task 并发数增加
* 负载上升与 pod 数变化近似同步
* 影响面更广，且更像阶跃突变

这就是反事实思路：
**假设为真时，世界还该呈现什么样子？**
如果它没呈现，那这个假设就弱。

---

## 四、拿你的案例完整跑一遍

### 现象

* DB CPU 从 50% 跳到 100%
* LLM 在代码和 SQL 上来回纠缠

### 初始假设

* H1：代码某处低效导致 CPU 升高
* H2：某条 SQL 变慢导致 CPU 升高

### Validator 开始审

#### 1）时间校验

查到：

* 10:20 发版
* 10:22 pod 从 1 → 8
* 10:23 全量任务开始
* 10:24 DB CPU 抬升

这时系统会说：

> H1/H2 目前还没解释“为什么偏偏 10:24 开始”；
> 发版/扩容/任务启动与异常开始时间高度邻近，必须纳入主因候选。

#### 2）量级校验

查到：

* 代码 diff 很小
* SQL 模式没发生巨大变化
* 单 pod 的理论额外负载不足以解释总 CPU +50%
* 但 pod 数翻了 8 倍，且全量任务是乘法放大器

系统会说：

> H1/H2 量级不足，最多解释一小部分；
> “8 个 pod 并发跑全量任务”在量级上更能解释 CPU 翻倍。

#### 3）范围校验

查到：

* 影响是整个实例组范围
* 多 pod 同时出现
* 不是单请求孤例

系统会说：

> 局部代码解释与全局范围不匹配，优先级下降。

#### 4）反事实校验

系统会进一步问：

* 如果只是代码小问题，为什么发版前没有出现同级异常？
* 如果只是 SQL 变慢，为什么异常与 pod 数变化同步？
* 如果真是并发任务放大，那是否有 job count / task start 证据？

然后去扩展世界，查：

* deploy history
* pod scaling
* job/task schedule
* 配置参数

最后得到更强解释：

> 不是“代码导致 CPU 爆掉”，
> 而是“发版导致 pod 扩增，8 个实例同时启动全量任务，负载成倍放大，把 DB CPU 顶满；代码层问题最多只是边际因子。”

这就是 Validator 的价值：
**不是更会讲故事，而是更会打假。**

---

## 五、怎么实现，才能应对复杂场景？

这里我直接给你工程方案。

### 方案核心：不要做成一个大模型 prompt，要做成“混合校验器”

最稳的是这套组合：

#### A. 规则校验

适合硬约束：

* 时间先后
* 同步性
* 范围匹配
* 缺失证据
* change point 附近是否有 deploy/config/scale 事件

这部分不要交给 LLM。

#### B. 统计校验

适合定量判断：

* 异常前后均值/中位数对比
* pod 数变化倍率
* QPS/CPU/任务数变化倍率
* 同期 SQL 占比变化
* change point detection

这部分用程序做，别让 LLM 猜。

#### C. LLM 校验

只让模型做它擅长的：

* 解释“这个假设为什么量级不够”
* 生成反事实检查项
* 把多源证据整合成可读结论
* 在证据不足时明确提出 unknowns

也就是说：

> **Validator 不是一个模型，而是一套审判流程。**

---

## 六、如何“保证”它能应对复杂场景？

这里我必须诚实一点：

> **不能保证。**
> 谁告诉你能保证，那就是在吹。

复杂事故里，不存在“永不失手”的 Validator。
你真正能做到的是三件事：

### 1）限制它的失败方式

让它即使错，也错得更安全。

比如：

* 不够证据时允许“不判”
* 不允许只因为看到代码 diff 就下主因结论
* 不允许忽略时间线和量级检查
* 不允许把局部解释直接套到全局异常上

### 2）让它更早暴露“不确定”

这很重要。

如果它发现：

* 时间线不完整
* 量级估计缺失
* 世界还没扩展到 deploy/config/job

那它应该输出：

> 当前无法判定主因，优先扩展世界到发布/扩缩容/任务调度面。

这比瞎猜强太多。

### 3）用 replay + case library 逼它成长

你真正能让它适应复杂场景的方法，不是写一条超级 prompt，而是建立 case 集合：

* 代码小问题但不是主因
* 配置切换导致全局异常
* 扩容放大任务并发
* 上游抖动引发重试风暴
* 观测失真误导时间线

然后每个 case 都测：

* 它有没有做时间校验
* 有没有做量级校验
* 有没有触发世界扩展
* 有没有把“次要因素”误判为“主因”

这才是“能应对复杂场景”的真正答案。

---

## 七、在 OpenClaw 里怎么落？

非常适合做成一个单独的 skill/tool 组合，而不是只靠主 skill 自由发挥。

我建议拆成 3 个工具：

* `validate_temporal_consistency`
* `validate_magnitude_fit`
* `validate_scope_fit`

再加一个汇总器：

* `validate_hypothesis`

输入是：

* hypothesis
* evidence pack
* world summaries（deploy/config/job/metrics）

输出是：

```json
{
  "hypothesis": "代码小问题导致DB CPU爆满",
  "temporal_fit": 0.35,
  "magnitude_fit": 0.10,
  "scope_fit": 0.20,
  "counterfactual_notes": [
    "无法解释为何异常在发版后立刻出现",
    "无法解释为何8个pod同步异常",
    "预期影响量级明显小于观测量级"
  ],
  "decision": "reject_as_primary_cause",
  "next_worlds_to_query": ["deploy", "scaling", "job"]
}
```

这就很像一个真正的“假设审判器”了。

---

## 最后一句，帮你定住这个模块的灵魂

> **Hypothesis Validator 的职责，不是帮助模型更自信。**
> **而是逼模型在时间、量级、范围面前收敛谦卑。**

这才是它在复杂场景里真正有价值的地方。
也是它区别于“再来一轮 LLM 总结”的根本。

[1]: https://dl.acm.org/doi/10.1145/3627703.3629553?utm_source=chatgpt.com "Automatic Root Cause Analysis via Large Language ..."
[2]: https://sre.google/workbook/postmortem-culture/?utm_source=chatgpt.com "Postmortem Practices for Incident Management"
[3]: https://arxiv.org/html/2602.08804v1?utm_source=chatgpt.com "Root Cause Analysis Method Based on Large Language ..."
[4]: https://arxiv.org/abs/2506.02490?utm_source=chatgpt.com "Simplifying Root Cause Analysis in Kubernetes with StateGraph and LLM"
[5]: https://www.sciencedirect.com/science/article/abs/pii/S0957582025007359?utm_source=chatgpt.com "Change point detection of processes using graph neural ..."
[6]: https://arxiv.org/html/2509.14435v1?utm_source=chatgpt.com "The Integration of Causal-Counterfactual Reasoning into ..."
