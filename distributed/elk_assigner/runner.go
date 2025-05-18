package elk_assigner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"time"
)

// Handle 处理分区任务的主循环
func (m *Mgr) Handle(ctx context.Context) error {
	m.Logger.Infof("开始任务处理循环")

	for {
		select {
		case <-ctx.Done():
			m.Logger.Infof("任务处理循环停止 (上下文取消)")
			return ctx.Err()
		case <-m.workCtx.Done():
			m.Logger.Infof("任务处理循环停止 (工作上下文取消)")
			return nil
		default:
			m.executeTaskCycle(ctx)
		}
	}
}

// acquirePartitionTask 获取一个可用的分区任务
func (m *Mgr) acquirePartitionTask(ctx context.Context) (PartitionInfo, error) {
	// 获取当前分区信息
	partitions, err := m.getPartitions(ctx)
	if err != nil {
		return PartitionInfo{}, errors.Wrap(err, "获取分区信息失败")
	}

	if len(partitions) == 0 {
		return PartitionInfo{}, ErrNoAvailablePartition
	}

	// 寻找可用的pending分区
	for partitionID, partition := range partitions {
		if partition.Status == StatusPending && partition.WorkerID == "" {
			// 尝试锁定这个分区
			lockKey := fmt.Sprintf(PartitionLockFmtFmt, m.Namespace, partitionID)
			locked, err := m.acquirePartitionLock(ctx, lockKey, partitionID)
			if err != nil || !locked {
				continue
			}

			// 更新分区信息，标记为该节点处理
			partition.WorkerID = m.ID
			partition.Status = StatusClaimed
			partition.UpdatedAt = time.Now()

			if err := m.updatePartitionStatus(ctx, partition); err != nil {
				// 如果更新失败，释放锁
				m.DataStore.ReleaseLock(ctx, lockKey, m.ID)
				return PartitionInfo{}, errors.Wrap(err, "更新分区状态失败")
			}

			return partition, nil
		}
	}

	return PartitionInfo{}, ErrNoAvailablePartition
}

// acquirePartitionLock 尝试获取分区锁
func (m *Mgr) acquirePartitionLock(ctx context.Context, lockKey string, partitionID int) (bool, error) {
	success, err := m.DataStore.AcquireLock(ctx, lockKey, m.ID, m.PartitionLockExpiry)
	if err != nil {
		m.Logger.Warnf("获取分区 %d 锁失败: %v", partitionID, err)
		return false, err
	}

	if success {
		m.Logger.Infof("成功锁定分区 %d", partitionID)
		return true, nil
	}

	return false, nil
}

// getPartitions 获取当前所有分区信息
func (m *Mgr) getPartitions(ctx context.Context) (map[int]PartitionInfo, error) {
	partitionInfoKey := fmt.Sprintf(PartitionInfoKeyFmt, m.Namespace)
	partitionsData, err := m.DataStore.GetPartitions(ctx, partitionInfoKey)
	if err != nil {
		return nil, err
	}

	if partitionsData == "" {
		return nil, nil
	}

	var partitions map[int]PartitionInfo
	if err := json.Unmarshal([]byte(partitionsData), &partitions); err != nil {
		return nil, errors.Wrap(err, "解析分区数据失败")
	}

	return partitions, nil
}

// processPartitionTask 处理一个分区任务
func (m *Mgr) processPartitionTask(ctx context.Context, task PartitionInfo) error {
	m.Logger.Infof("开始处理分区 %d (ID范围: %d-%d)", task.PartitionID, task.MinID, task.MaxID)

	// 更新分区状态为运行中
	if err := m.updateTaskStatus(ctx, task, StatusRunning); err != nil {
		return errors.Wrap(err, "更新分区状态为running失败")
	}

	// 准备处理选项并执行任务
	processCount, err := m.executeProcessorTask(ctx, task)
	m.Logger.Infof("分区 %d 处理完成: 处理项数=%d, 错误=%v", task.PartitionID, processCount, err)

	// 根据处理结果更新状态
	newStatus := StatusCompleted
	if err != nil {
		newStatus = StatusFailed
	}

	// 更新任务状态并释放锁
	if updateErr := m.updateTaskStatus(ctx, task, newStatus); updateErr != nil {
		m.Logger.Errorf("更新分区 %d 状态为 %s 失败: %v",
			task.PartitionID, newStatus, updateErr)
	}

	m.releasePartitionLock(ctx, task.PartitionID)

	return err
}

// executeProcessorTask 执行处理器任务
func (m *Mgr) executeProcessorTask(ctx context.Context, task PartitionInfo) (int64, error) {
	// 准备处理选项
	options := task.Options
	if options == nil {
		options = make(map[string]interface{})
	}
	options["partition_id"] = task.PartitionID
	options["worker_id"] = m.ID

	// 处理分区数据
	return m.TaskProcessor.Process(ctx, task.MinID, task.MaxID, options)
}

// updateTaskStatus 更新任务状态
func (m *Mgr) updateTaskStatus(ctx context.Context, task PartitionInfo, status string) error {
	task.Status = status
	task.UpdatedAt = time.Now()
	return m.updatePartitionStatus(ctx, task)
}

// releasePartitionLock 释放分区锁
func (m *Mgr) releasePartitionLock(ctx context.Context, partitionID int) {
	lockKey := fmt.Sprintf(PartitionLockFmtFmt, m.Namespace, partitionID)
	if releaseErr := m.DataStore.ReleaseLock(ctx, lockKey, m.ID); releaseErr != nil {
		m.Logger.Warnf("释放分区 %d 锁失败: %v", partitionID, releaseErr)
	}
}

// updatePartitionStatus 更新分区状态
func (m *Mgr) updatePartitionStatus(ctx context.Context, task PartitionInfo) error {
	// 获取当前所有分区
	partitions, err := m.getPartitions(ctx)
	if err != nil {
		return errors.Wrap(err, "获取分区信息失败")
	}

	// 更新特定分区
	partitions[task.PartitionID] = task

	// 保存更新后的分区信息
	return m.savePartitions(ctx, partitions)
}

// savePartitions 保存分区信息到存储
func (m *Mgr) savePartitions(ctx context.Context, partitions map[int]PartitionInfo) error {
	partitionInfoKey := fmt.Sprintf(PartitionInfoKeyFmt, m.Namespace)
	updatedData, err := json.Marshal(partitions)
	if err != nil {
		return errors.Wrap(err, "编码更新后分区数据失败")
	}

	return m.DataStore.SetPartitions(ctx, partitionInfoKey, string(updatedData))
}

// executeTaskCycle 执行单个任务处理周期
func (m *Mgr) executeTaskCycle(ctx context.Context) {
	// 获取分区任务
	// 小心获取分区任务时的分布式竞争问题
	task, err := m.acquirePartitionTask(ctx)

	if err == nil {
		// 处理分区任务
		m.Logger.Infof("获取到分区任务 %d, 范围 [%d, %d]",
			task.PartitionID, task.MinID, task.MaxID)

		if processErr := m.processPartitionTask(ctx, task); processErr != nil {
			m.handleTaskError(ctx, task, processErr)
		}

		// 处理完成后休息一下，避免立即抢占下一个任务
		time.Sleep(taskCompletedDelay)
	} else {
		m.handleAcquisitionError(err)
	}
}

// handleTaskError 处理任务执行错误
func (m *Mgr) handleTaskError(ctx context.Context, task PartitionInfo, processErr error) {
	m.Logger.Errorf("处理分区 %d 失败: %v", task.PartitionID, processErr)

	// 更新分区状态为失败
	task.Status = StatusFailed
	if updateErr := m.updatePartitionStatus(ctx, task); updateErr != nil {
		m.Logger.Warnf("更新分区状态失败: %v", updateErr)
	}
}

// handleAcquisitionError 处理任务获取错误
func (m *Mgr) handleAcquisitionError(err error) {
	if errors.Is(err, ErrNoAvailablePartition) {
		// 没有可用分区，等待一段时间再检查
		time.Sleep(noTaskDelay)
	} else {
		// 其他错误
		m.Logger.Warnf("获取分区任务失败: %v", err)
		time.Sleep(taskRetryDelay)
	}
}
