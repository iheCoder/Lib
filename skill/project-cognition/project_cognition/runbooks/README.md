# Runbooks（操作经验）

## 这一层存什么

- 操作步骤：如何执行某个脚本、如何触发某个流程
- 故障修复：特定 Bug 的数据修复方法、回滚步骤
- 巡检流程：定期需要检查什么、怎么检查
- 环境配置：开发/测试/生产环境的关键差异和配置点
- 踩坑记录：执行某操作时已知的坑和规避方式

## 不存什么

- 业务链路或系统架构描述 → 放 `domain-knowledge/`
- 项目原则或策略 → 放 `core-manifest/`
- 纯代码（代码放仓库，runbook 引用路径即可）

## 文件命名

```
<场景>-<操作>.md
```

示例：
- `redis-key-repair.md`
- `kafka-consumer-lag-check.md`
- `deploy-rollback.md`
- `user-data-backfill.md`
- `on-call-checklist.md`

## 新增 vs 更新

- 已有文件覆盖同一操作：在该文件内追加新版本的步骤，保留旧步骤（标注已废弃）
- 操作步骤有重大变化：新建版本段落 `## v2（YYYY-MM-DD）`
- 踩到新坑：在「已知失败信号」或「踩坑记录」小节追加

## 使用模板

新建文件时请复制 `_template.md` 并填写全部必填字段。
