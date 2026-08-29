# Requirement-Scoped Dry-Run Cases

这些 case 用于检查 Side Eye V2 的行为，而不是检查它是否复述固定标题。运行 review 时只给被测 reviewer 当前 case 的 `Requirement / Seed code / Runtime context`；不要给它看 `dry-run-report.md`。

## Case 1: Ordinary business patch must stay ordinary

### Requirement

管理员可以只修改客户通知邮箱；未在请求中出现的短信与营销偏好必须保持原值。成功响应后读取接口应返回新邮箱。

### Seed code

```go
// notification_service.go
func (s *Service) UpdateEmail(ctx context.Context, userID string, req UpdateEmailRequest) error {
    patch := Preferences{Email: req.Email}
    return s.repo.SavePreferences(ctx, userID, patch)
}

// preferences_repo.go
func (r *Repo) SavePreferences(ctx context.Context, userID string, value Preferences) error {
    _, err := r.db.ExecContext(ctx, `
        UPDATE preferences
        SET email = ?, sms_enabled = ?, marketing_enabled = ?
        WHERE user_id = ?`,
        value.Email, value.SMSEnabled, value.MarketingEnabled, userID)
    return err
}
```

### Runtime context

`sms_enabled` 和 `marketing_enabled` 已有真实用户数据。Seed 之外还有大量无关 working-tree changes。

## Case 2: Repair can overwrite fresh state

### Requirement

新增周期性 repair job，把数据库中的账户限额同步到运行时 quota store；repair 与正常更新接口会同时在线运行。

### Seed code

```go
// quota_repair.go
func (j *RepairJob) Run(ctx context.Context) error {
    snapshot, err := j.accounts.ListQuotaSnapshot(ctx)
    if err != nil { return err }
    for _, account := range snapshot {
        if err := j.quota.Set(ctx, account.ID, account.Limit); err != nil { return err }
    }
    return nil
}

// quota_service.go
func (s *Service) UpdateLimit(ctx context.Context, id string, limit int64) error {
    if err := s.accounts.UpdateLimit(ctx, id, limit); err != nil { return err }
    return s.quota.Set(ctx, id, limit)
}
```

### Runtime context

`ListQuotaSnapshot` 是一次长达数分钟的可重复读快照；quota store 不保存 DB version，也不做条件写。

## Case 3: Agent says modified, scheduler still runs old task

### Requirement

用户可以通过多轮自然语言对话修改内置任务的执行时间；修改必须真实反映到下一次调度，最终回复才能声称成功。

### Seed code

```go
// modify_builtin_task_tool.go
func (t *Tool) Execute(ctx context.Context, args ModifyArgs) (ToolResult, error) {
    if err := t.tasks.UpdateSchedule(ctx, args.TaskID, args.Schedule); err != nil {
        return ToolResult{}, err
    }
    return ToolResult{Message: "修改成功"}, nil
}

// scheduler.go
func (s *Scheduler) Start(ctx context.Context) error {
    tasks, err := s.tasks.ListEnabled(ctx)
    if err != nil { return err }
    s.entries = buildEntries(tasks)
    return s.loop(ctx)
}
```

### Runtime context

Scheduler 是常驻进程，`Start` 只在进程启动时执行。用户把每天 9 点改成每天 10 点。

## Case 4: Approval no longer matches executed action

### Requirement

Agent 修改生产任务前必须展示目标任务与新时间，得到用户批准后只能执行这一个已批准动作。

### Seed code

```go
// planner.go
func (p *Planner) Draft(ctx context.Context, query string) (Draft, error) {
    matches, err := p.tasks.Search(ctx, query)
    if err != nil { return Draft{}, err }
    return Draft{CandidateIndex: 0, NewTime: "10:00"}, nil
}

// approval.go
func (a *Approvals) Approve(sessionID string) string {
    return sign(sessionID)
}

// executor.go
func (e *Executor) Execute(ctx context.Context, sessionID string, draft Draft, token string) error {
    if !verify(token, sessionID) { return ErrNotApproved }
    matches, err := e.tasks.Search(ctx, e.session.Query(sessionID))
    if err != nil { return err }
    return e.tasks.UpdateTime(ctx, matches[draft.CandidateIndex].ID, draft.NewTime)
}
```

### Runtime context

审批后、执行前，任务搜索结果可能因重命名或新增任务改变排序。审批 token 只绑定 session ID。

## Case 5: Live backfill meets old writers

### Requirement

为任务增加规范化 `status_v2`，发布过程中 backfill 历史记录；系统采用 rolling deployment，旧实例会继续处理更新请求。

### Seed code

```sql
-- backfill.sql
UPDATE tasks
SET status_v2 = CASE
    WHEN completed_at IS NOT NULL THEN 'completed'
    WHEN disabled = 1 THEN 'disabled'
    ELSE 'active'
END
WHERE status_v2 IS NULL;
```

```go
// v1_task_writer.go
func (s *Service) Disable(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE tasks SET disabled = 1, completed_at = NULL WHERE id = ?`, id)
    return err
}
```

### Runtime context

Backfill 扫描需要数小时；v1 writer 不写 `status_v2`，v2 reader 优先读取非空 `status_v2`。部署顺序尚未证明会先停止 v1 writer。

## Negative controls

每次 dry-run 还要检查没有产生以下无关结论：

- 单实例、仅承载可容忍陈旧展示数据的 Redis cache，不应触发 lease/fencing；
- 从未发布、无消费者也无历史数据的 API，不应触发 mixed-version compatibility；
- 固定执行 3 次的 loop，不应触发 cross-layer amplification；
- Agent 成功响应前已 read-back 验证真实环境状态时，不应泛泛报告 completion claim mismatch。
