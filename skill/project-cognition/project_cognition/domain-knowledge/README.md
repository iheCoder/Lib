# Domain Knowledge（领域知识）

## 这一层存什么

- 业务链路：数据从哪里来、经过哪些系统、最终落在哪里
- 系统关系：各服务/组件之间的依赖和调用关系
- 关键字段：核心指标的口径、来源、计算逻辑
- 异常点：已知的数据问题、边界 case、常见误区
- 背景知识：理解某个系统/模块必须了解的前置概念

## 不存什么

- 操作命令或脚本步骤 → 放 `runbooks/`
- 项目原则或禁区 → 放 `core-manifest/`
- 单次调试日志（可以总结后写入，但不要原样粘贴）

## 文件命名

```
<系统/子系统/模块>-<主题>.md
```

示例：
- `user-profile-pipeline.md`
- `kafka-hologres-redis-data-flow.md`
- `order-key-metrics.md`
- `recommendation-system-overview.md`

一个文件对应一个聚焦主题；主题过大时按子系统拆分。

## 目录结构建议

如果领域知识很多，可以在 `domain-knowledge/` 下建子目录：

```
domain-knowledge/
├── user/
├── order/
├── data-pipeline/
└── infra/
```

子目录内同样使用 `_template.md`。

## 新增 vs 更新

- 已有文件覆盖同一系统：在该文件内增量追加新的小节，保留原有内容
- 发现已有内容有误：在原内容下方追加 `> [修正 YYYY-MM-DD]` 块，不要直接删改
- 信息来源不确定：标记 `[待确认]`

## 使用模板

新建文件时请复制 `_template.md` 并填写全部必填字段。
