# Core Manifest（规则层）

## 这一层存什么

- 项目级原则和不变量
- 修改策略（哪些可以改、哪些需要审批、哪些绝对禁止）
- 禁区：不能动的代码路径、配置、行为
- 验证要求：改动前必须满足的前置条件
- 架构决策记录（ADR）中的关键约束

## 不存什么

- 具体的业务链路或字段口径 → 放 `domain-knowledge/`
- 操作命令或脚本步骤 → 放 `runbooks/`
- 临时的实验性结论（未验证的放 `draft`，验证后升级为 `verified`）

## 文件命名

```
<主题>.md
```

示例：
- `api-versioning-policy.md`
- `database-schema-constraints.md`
- `deployment-gates.md`
- `global-principles.md`

## 新增 vs 更新

- 同一主题已有文件：在该文件内追加/修改，不要新建
- 主题跨多个系统：可按子系统拆分文件，通过 `Related Docs` 互链
- 发现规则被违反：在对应文件中追加「已知违规」小节，注明原因和状态

## 交叉引用

- 引用 domain knowledge：`@../domain-knowledge/<file>.md`
- 引用 runbooks：`@../runbooks/<file>.md`
- 引用仓库内文档：使用仓库相对路径

## 使用模板

新建文件时请复制 `_template.md` 并填写全部必填字段。
