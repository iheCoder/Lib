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
			// 获取分区任务
			// 小心获取分区任务时的分布式竞争问题
			task, err := m.acquirePartitionTask(ctx)

			if err == nil {
				// 处理分区任务
				m.Logger.Infof("获取到分区任务 %d, 范围 [%d, %d]",
					task.PartitionID, task.MinID, task.MaxID)

				processErr := m.processPartitionTask(ctx, task)
				if processErr != nil {
					m.Logger.Errorf("处理分区 %d 失败: %v", task.PartitionID, processErr)

					// 更新分区状态为失败
					task.Status = "failed"
					updateErr := m.updatePartitionStatus(ctx, task)
					if updateErr != nil {
						m.Logger.Warnf("更新分区状态失败: %v", updateErr)
					}
				}

				// 处理完成后休息一下，避免立即抢占下一个任务
				time.Sleep(500 * time.Millisecond)
			} else if errors.Is(err, ErrNoAvailablePartition) {
				// 没有可用分区，等待一段时间再检查
				time.Sleep(2 * time.Second)
			} else {
				// 其他错误
				m.Logger.Warnf("获取分区任务失败: %v", err)
				time.Sleep(1 * time.Second)
			}
		}
	}
}

// acquirePartitionTask 获取一个可用的分区任务
func (m *Mgr) acquirePartitionTask(ctx context.Context) (PartitionInfo, error) {
	// 获取当前分区信息
	partitionInfoKey := fmt.Sprintf(PartitionInfoKeyFmt, m.Namespace)
	partitionsData, err := m.DataStore.GetPartitions(ctx, partitionInfoKey)
	if err != nil {
		return PartitionInfo{}, errors.Wrap(err, "获取分区信息失败")
	}

	if partitionsData == "" {
		return PartitionInfo{}, ErrNoAvailablePartition
	}

	var partitions map[int]PartitionInfo
	if err := json.Unmarshal([]byte(partitionsData), &partitions); err != nil {
		return PartitionInfo{}, errors.Wrap(err, "解析分区数据失败")
	}

	if len(partitions) == 0 {
		return PartitionInfo{}, ErrNoAvailablePartition
	}

	// 寻找可用的pending分区
	for partitionID, partition := range partitions {
		if partition.Status == "pending" && partition.WorkerID == "" {
			// 尝试锁定这个分区
			lockKey := fmt.Sprintf(PartitionLockFmtFmt, m.Namespace, partitionID)
			success, err := m.DataStore.AcquireLock(ctx, lockKey, m.ID, m.PartitionLockExpiry)
			if err != nil {
				m.Logger.Warnf("获取分区 %d 锁失败: %v", partitionID, err)
				continue
			}

			if success {
				m.Logger.Infof("成功锁定分区 %d", partitionID)

				// 更新分区信息，标记为该节点处理
				partition.WorkerID = m.ID
				partition.Status = "claimed"
				partition.UpdatedAt = time.Now()

				if err := m.updatePartitionStatus(ctx, partition); err != nil {
					// 如果更新失败，释放锁
					m.DataStore.ReleaseLock(ctx, lockKey, m.ID)
					return PartitionInfo{}, errors.Wrap(err, "更新分区状态失败")
				}

				return partition, nil
			}
		}
	}

	return PartitionInfo{}, ErrNoAvailablePartition
}

// processPartitionTask 处理一个分区任务
func (m *Mgr) processPartitionTask(ctx context.Context, task PartitionInfo) error {
	m.Logger.Infof("开始处理分区 %d (ID范围: %d-%d)", task.PartitionID, task.MinID, task.MaxID)

	// 更新分区状态为运行中
	task.Status = "running"
	if err := m.updatePartitionStatus(ctx, task); err != nil {
		return errors.Wrap(err, "更新分区状态为running失败")
	}

	// 准备处理选项
	options := task.Options
	if options == nil {
		options = make(map[string]interface{})
	}
	options["partition_id"] = task.PartitionID
	options["worker_id"] = m.ID

	// 处理分区数据
	processCount, err := m.TaskProcessor.Process(ctx, task.MinID, task.MaxID, options)
	m.Logger.Infof("分区 %d 处理完成: 处理项数=%d, 错误=%v", task.PartitionID, processCount, err)

	// 根据处理结果更新状态
	if err != nil {
		task.Status = "failed"
	} else {
		task.Status = "completed"
	}

	// 更新分区状态
	updateErr := m.updatePartitionStatus(ctx, task)
	if updateErr != nil {
		m.Logger.Errorf("更新分区 %d 状态为 %s 失败: %v", task.PartitionID, task.Status, updateErr)
	}

	// 释放分区锁
	lockKey := fmt.Sprintf(PartitionLockFmtFmt, m.Namespace, task.PartitionID)
	if releaseErr := m.DataStore.ReleaseLock(ctx, lockKey, m.ID); releaseErr != nil {
		m.Logger.Warnf("释放分区 %d 锁失败: %v", task.PartitionID, releaseErr)
	}

	return err
}

// updatePartitionStatus 更新分区状态
func (m *Mgr) updatePartitionStatus(ctx context.Context, task PartitionInfo) error {
	// 获取当前所有分区
	partitionInfoKey := fmt.Sprintf(PartitionInfoKeyFmt, m.Namespace)
	partitionsData, err := m.DataStore.GetPartitions(ctx, partitionInfoKey)
	if err != nil {
		return errors.Wrap(err, "获取分区信息失败")
	}

	var partitions map[int]PartitionInfo
	if err := json.Unmarshal([]byte(partitionsData), &partitions); err != nil {
		return errors.Wrap(err, "解析分区数据失败")
	}

	// 更新特定分区
	task.UpdatedAt = time.Now()
	partitions[task.PartitionID] = task

	// 保存更新后的分区信息
	updatedData, err := json.Marshal(partitions)
	if err != nil {
		return errors.Wrap(err, "编码更新后分区数据失败")
	}

	return m.DataStore.SetPartitions(ctx, partitionInfoKey, string(updatedData))
}
