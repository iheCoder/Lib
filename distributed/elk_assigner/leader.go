package elk_assigner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"time"
)

// Lead 执行Leader相关的工作
func (m *Mgr) Lead(ctx context.Context) error {
	ticker := time.NewTicker(m.LeaderElectionInterval)
	defer ticker.Stop()

	// 尝试竞选leader
	elected, err := m.tryElect(ctx)
	if err != nil {
		m.Logger.Warnf("竞选Leader失败: %v", err)
	}

	// 竞选成功，则分配分区
	if elected {
		m.leaderCtx, m.cancelLeader = context.WithCancel(context.Background())
		m.Logger.Infof("节点 %s 成为Leader，开始Leader工作", m.ID)
		go m.doLeaderWork(ctx)
	}

	// 竞选失败，则周期性判断是否需要重新竞选
	for {
		select {
		case <-ctx.Done():
			m.Logger.Infof("Leader选举任务停止 (上下文取消)")
			return ctx.Err()
		case <-ticker.C:
			// 如果当前不是Leader，尝试重新竞选
			m.mu.RLock()
			isLeader := m.isLeader
			m.mu.RUnlock()

			if !isLeader {
				elected, err := m.tryElect(ctx)
				if err != nil {
					m.Logger.Warnf("重新竞选Leader失败: %v", err)
					continue
				}

				if elected {
					m.leaderCtx, m.cancelLeader = context.WithCancel(context.Background())
					m.Logger.Infof("节点 %s 成为Leader，开始Leader工作", m.ID)
					go m.doLeaderWork(ctx)
				}
			}
		}
	}
}

// tryElect 尝试竞选Leader
func (m *Mgr) tryElect(ctx context.Context) (bool, error) {
	// 竞选leader
	// 1. 获取leader锁
	leaderLockKey := fmt.Sprintf(LeaderLockKeyFmt, m.Namespace)
	success, err := m.DataStore.AcquireLock(ctx, leaderLockKey, m.ID, m.LeaderLockExpiry)
	if err != nil {
		return false, errors.Wrap(err, "获取Leader锁失败")
	}

	// 2. 如果获取锁成功，则成为leader
	if success {
		m.Logger.Infof("成功获取Leader锁，节点 %s 成为Leader", m.ID)

		m.mu.Lock()
		m.isLeader = true
		m.mu.Unlock()

		return true, nil
	}

	// 3. 否则，竞选失败
	m.Logger.Debugf("获取Leader锁失败，将作为Worker运行")
	return false, nil
}

// doLeaderWork 执行Leader的工作
func (m *Mgr) doLeaderWork(ctx context.Context) error {
	// 1. 异步周期性续leader锁
	go m.renewLeaderLock()

	// 2. 持续性分配分区
	ticker := time.NewTicker(m.LeaderElectionInterval / 2)
	defer ticker.Stop()

	// 从lastProcessedID开始，持续分配分区任务，直到GetNextMaxID没有更多任务为止
	for {
		select {
		case <-ctx.Done():
			m.Logger.Infof("Leader工作停止 (上下文取消)")
			return ctx.Err()
		case <-m.leaderCtx.Done():
			m.Logger.Infof("Leader工作停止 (不再是Leader)")
			return nil
		case <-ticker.C:
			if err := m.allocatePartitions(ctx); err != nil {
				m.Logger.Warnf("分配分区失败: %v", err)
			}
		}
	}
}

// renewLeaderLock 定期更新Leader锁
func (m *Mgr) renewLeaderLock() {
	leaderLockKey := fmt.Sprintf(LeaderLockKeyFmt, m.Namespace)
	ticker := time.NewTicker(m.LeaderLockExpiry / 3)
	defer ticker.Stop()

	for {
		select {
		case <-m.leaderCtx.Done():
			m.Logger.Infof("停止更新Leader锁")
			return
		case <-ticker.C:
			ctx := context.Background()
			success, err := m.DataStore.RenewLock(ctx, leaderLockKey, m.ID, m.LeaderLockExpiry)
			if err != nil {
				m.Logger.Warnf("更新Leader锁失败: %v", err)
				continue
			}

			if !success {
				m.Logger.Warnf("无法更新Leader锁，可能锁已被其他节点获取")

				m.mu.Lock()
				m.isLeader = false
				m.mu.Unlock()

				if m.cancelLeader != nil {
					m.cancelLeader()
				}

				return
			}
		}
	}
}

// allocatePartitions 分配工作分区
func (m *Mgr) allocatePartitions(ctx context.Context) error {
	// 获取当前已处理的最大ID
	lastProcessedID, err := m.TaskProcessor.GetLastProcessedID(ctx)
	if err != nil {
		return errors.Wrap(err, "获取最后处理的ID失败")
	}

	// 获取下一批次的最大ID
	nextMaxID, err := m.TaskProcessor.GetNextMaxID(ctx, lastProcessedID, 10000) // 使用一个合理的范围大小
	if err != nil {
		return errors.Wrap(err, "获取下一个最大ID失败")
	}

	// 如果没有新的数据要处理，直接返回
	if nextMaxID <= lastProcessedID {
		m.Logger.Debugf("没有新的数据需要处理，当前最大ID: %d", lastProcessedID)
		return nil
	}

	// 获取活跃节点
	activeWorkers, err := m.getActiveWorkers(ctx)
	if err != nil {
		return errors.Wrap(err, "获取活跃节点失败")
	}

	// 计算分区数量
	partitionCount := len(activeWorkers)
	if partitionCount == 0 {
		partitionCount = 1 // 至少有一个分区
	} else if partitionCount > DefaultPartitionCount {
		partitionCount = DefaultPartitionCount // 限制最大分区数
	}

	// 计算每个分区的ID范围
	partitionSize := (nextMaxID - lastProcessedID) / int64(partitionCount)
	if partitionSize < 1000 {
		partitionSize = 1000 // 确保每个分区至少有1000个ID
	}

	// 创建分区
	partitions := make(map[int]PartitionInfo)
	for i := 0; i < partitionCount; i++ {
		minID := lastProcessedID + int64(i)*partitionSize + 1
		maxID := lastProcessedID + int64(i+1)*partitionSize

		// 最后一个分区处理到nextMaxID
		if i == partitionCount-1 {
			maxID = nextMaxID
		}

		partitions[i] = PartitionInfo{
			PartitionID: i,
			MinID:       minID,
			MaxID:       maxID,
			WorkerID:    "", // 空，等待被认领
			Status:      "pending",
			UpdatedAt:   time.Now(),
			Options:     make(map[string]interface{}),
		}
	}

	// 保存分区到存储
	partitionInfoKey := fmt.Sprintf(PartitionInfoKeyFmt, m.Namespace)
	partitionsData, err := json.Marshal(partitions)
	if err != nil {
		return errors.Wrap(err, "序列化分区数据失败")
	}

	err = m.DataStore.SetPartitions(ctx, partitionInfoKey, string(partitionsData))
	if err != nil {
		return errors.Wrap(err, "保存分区数据失败")
	}

	m.Logger.Infof("成功创建 %d 个分区，ID范围 [%d, %d]", partitionCount, lastProcessedID+1, nextMaxID)

	return nil
}
