# 三层写法示例

本文档通过三个具体样例，帮助 Agent 快速判断内容应该写在哪一层。

---

## 示例 1：Core Manifest

**场景**：你了解到「生产数据库 Schema 不允许直接在线上 ALTER TABLE，必须走数据库变更流程」。

**这是规则 → 写入 `core-manifest/`**

文件：`core-manifest/database-schema-constraints.md`

```markdown
---
status: verified
scope: 全局
source_of_truth: 工程规范 Wiki > 数据库变更流程
last_reviewed: 2026-03-20
when_to_update: 变更流程调整时
---

# 数据库 Schema 变更约束

## 原则

- 禁止在生产环境直接执行 ALTER TABLE
- 所有 Schema 变更必须通过变更工单审批后由 DBA 执行

## 禁区

- [ ] 禁止：直接 SSH 到生产数据库执行 DDL（原因：风险不可控，无审计记录）

## 修改策略

1. 在测试环境验证变更脚本
2. 提交变更工单，填写影响范围和回滚方案
3. DBA 审批后在低峰期执行
```

---

## 示例 2：Domain Knowledge

**场景**：你了解到「用户画像数据走 Kafka → Hologres → Redis 链路，其中 `user_level` 字段是从 Hologres 计算后写入 Redis 的」。

**这是系统链路和字段口径 → 写入 `domain-knowledge/`**

文件：`domain-knowledge/user-profile-pipeline.md`

```markdown
---
status: verified
scope: 用户系统
source_of_truth: 用户画像服务代码 user-profile-service/pipeline/
last_reviewed: 2026-03-20
when_to_update: 链路变更时、字段口径调整时
---

# 用户画像数据链路

## 链路

```
Kafka(行为事件) → Hologres(宽表聚合) → Redis(热数据)
```

| 节点 | 职责 | 延迟 |
|------|------|------|
| Kafka | 接收用户行为事件 | 实时 |
| Hologres | 聚合计算用户画像字段 | 分钟级 |
| Redis | 缓存热点用户数据供 API 读取 | 秒级 |

## 关键字段

| 字段名 | 含义 | 来源 | 计算逻辑 |
|--------|------|------|----------|
| `user_level` | 用户等级 | Hologres → Redis | 按过去 30 天消费金额分层，规则详见 Wiki |

## 已知异常点

- **问题**：Redis 缓存 TTL 默认 1 小时，`user_level` 变更后最多延迟 1 小时才能反映
  **规避**：需要实时生效时，手动 DEL 对应 key 触发刷新
```

---

## 示例 3：Runbooks

**场景**：某批用户的 `user_level` 因数据管道异常被写成了错误值，需要手动修复 Redis 中的数据。

**这是数据修复操作 → 写入 `runbooks/`**

文件：`runbooks/user-level-redis-repair.md`

```markdown
---
status: verified
scope: 生产
source_of_truth: 修复脚本 scripts/repair/fix_user_level.py
last_reviewed: 2026-03-20
when_to_update: 修复脚本变更时
related_docs:
  - ../domain-knowledge/user-profile-pipeline.md
---

# user_level Redis 数据修复

## 适用场景

- 数据管道异常导致 Redis 中 `user_level` 被写入错误值
- 需要针对特定用户批量修复

## 前置条件

- [ ] 已确认 Hologres 中正确的 `user_level` 值
- [ ] 已获取生产 Redis 写权限
- [ ] 影响范围已评估（影响用户 UID 列表已准备好）

## 操作步骤

### 第 1 步：生成修复列表

```bash
python3 scripts/repair/fix_user_level.py --dry-run --uid-file uid_list.txt
```

**验证**：输出日志中 `[DRY RUN]` 行数与预期用户数一致

### 第 2 步：执行修复

```bash
python3 scripts/repair/fix_user_level.py --uid-file uid_list.txt
```

**验证**：查询几个典型 UID 的 Redis key，确认值已更新

## 验证方法

```bash
redis-cli GET user:profile:{uid}
```

预期：`user_level` 字段值与 Hologres 查询结果一致

## 已知失败信号

| 信号 | 含义 | 处理方式 |
|------|------|----------|
| `NOAUTH` 错误 | Redis 认证失败 | 检查 REDIS_PASSWORD 环境变量 |
| 修复后 level 仍错 | TTL 未到期，被旧值覆盖 | DEL key 后重试 |
```

---

## 快速分类检查表

| 内容类型 | 目标层 |
|---------|--------|
| "禁止 XXX" / "必须 XXX" | core-manifest |
| "系统 A 的数据来自系统 B" | domain-knowledge |
| "字段 X 的口径是…" | domain-knowledge |
| "执行命令 `xxx` 来修复…" | runbooks |
| "遇到报错 Y，应该这样处理" | runbooks |
| "架构决策：选择了 X 而非 Y" | core-manifest |
