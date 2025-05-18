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

	// 初次尝试竞选leader
	if elected := m.initialElection(ctx); elected {
		m.startLeaderWork(ctx)
	}

	// 竞选失败，则周期性判断是否需要重新竞选
	for {
		select {
		case <-ctx.Done():
			m.Logger.Infof("Leader选举任务停止 (上下文取消)")
			return ctx.Err()
		case <-ticker.C:
			m.periodicElection(ctx)
		}
	}
}

// initialElection 进行初始Leader选举
func (m *Mgr) initialElection(ctx context.Context) bool {
	elected, err := m.tryElect(ctx)
	if err != nil {
		m.Logger.Warnf("竞选Leader失败: %v", err)
		return false
	}
	return elected
}

// periodicElection 周期性检查是否需要重新竞选
func (m *Mgr) periodicElection(ctx context.Context) {
	// 如果当前不是Leader，尝试重新竞选
	m.mu.RLock()
	isLeader := m.isLeader
	m.mu.RUnlock()

	if !isLeader {
		elected, err := m.tryElect(ctx)
		if err != nil {
			m.Logger.Warnf("重新竞选Leader失败: %v", err)
			return
		}

		if elected {
			m.startLeaderWork(ctx)
		}
	}
}

// startLeaderWork 启动Leader工作
func (m *Mgr) startLeaderWork(ctx context.Context) {
	m.leaderCtx, m.cancelLeader = context.WithCancel(context.Background())
	m.Logger.Infof("节点 %s 成为Leader，开始Leader工作", m.ID)
	go m.doLeaderWork(ctx)
}

// tryElect 尝试竞选Leader
func (m *Mgr) tryElect(ctx context.Context) (bool, error) {
	// 获取leader锁
	leaderLockKey := fmt.Sprintf(LeaderLockKeyFmt, m.Namespace)
	success, err := m.DataStore.AcquireLock(ctx, leaderLockKey, m.ID, m.LeaderLockExpiry)
	if err != nil {
		return false, errors.Wrap(err, "获取Leader锁失败")
	}

	if success {
		m.becomeLeader()
		return true, nil
	}

	// 竞选失败
	m.Logger.Debugf("获取Leader锁失败，将作为Worker运行")
	return false, nil
}

// becomeLeader 设置当前节点为Leader
func (m *Mgr) becomeLeader() {
	m.Logger.Infof("成功获取Leader锁，节点 %s 成为Leader", m.ID)

	m.mu.Lock()
	m.isLeader = true
	m.mu.Unlock()
}

// doLeaderWork 执行Leader的工作
func (m *Mgr) doLeaderWork(ctx context.Context) error {
	// 1. 异步周期性续leader锁
	go m.renewLeaderLock()

	// 2. 持续性分配分区
	return m.runPartitionAllocationLoop(ctx)
}

// runPartitionAllocationLoop 运行分区分配循环
func (m *Mgr) runPartitionAllocationLoop(ctx context.Context) error {
	// 使用Leader选举间隔的一半作为分区分配频率
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
			m.tryAllocatePartitions(ctx)
		}
	}
}

// tryAllocatePartitions 尝试分配分区并处理错误
func (m *Mgr) tryAllocatePartitions(ctx context.Context) {
	if err := m.allocatePartitions(ctx); err != nil {
		m.Logger.Warnf("分配分区失败: %v", err)
	}
}

// renewLeaderLock 定期更新Leader锁
func (m *Mgr) renewLeaderLock() {
	leaderLockKey := fmt.Sprintf(LeaderLockKeyFmt, m.Namespace)
	// 使锁更新频率是锁超时时间的1/3，确保有足够时间进行更新
	ticker := time.NewTicker(m.LeaderLockExpiry / 3)
	defer ticker.Stop()

	for {
		select {
		case <-m.leaderCtx.Done():
			m.Logger.Infof("停止更新Leader锁")
			return
		case <-ticker.C:
			if !m.tryRenewLeaderLock(leaderLockKey) {
				m.relinquishLeadership()
				return
			}
		}
	}
}

// tryRenewLeaderLock 尝试更新Leader锁
func (m *Mgr) tryRenewLeaderLock(leaderLockKey string) bool {
	ctx := context.Background()
	success, err := m.DataStore.RenewLock(ctx, leaderLockKey, m.ID, m.LeaderLockExpiry)

	if err != nil {
		m.Logger.Warnf("更新Leader锁失败: %v", err)
		return true // 遇到错误时继续尝试，不放弃领导权
	}

	if !success {
		m.Logger.Warnf("无法更新Leader锁，可能锁已被其他节点获取")
		return false // 更新失败，需要放弃领导权
	}

	return true // 成功更新，继续保持领导权
}

// relinquishLeadership 放弃领导权
func (m *Mgr) relinquishLeadership() {
	m.mu.Lock()
	m.isLeader = false
	m.mu.Unlock()

	if m.cancelLeader != nil {
		m.cancelLeader()
	}
}

// allocatePartitions 分配工作分区
func (m *Mgr) allocatePartitions(ctx context.Context) error {
	// 获取ID范围
	lastProcessedID, nextMaxID, err := m.getProcessingRange(ctx)
	if err != nil {
		return err
	}

	// 如果没有新的数据要处理，直接返回
	if nextMaxID <= lastProcessedID {
		m.Logger.Debugf("没有新的数据需要处理，当前最大ID: %d", lastProcessedID)
		return nil
	}

	// 计算分区数量和大小
	partitionCount, err := m.calculatePartitionCount(ctx)
	if err != nil {
		return err
	}

	// 创建分区
	partitions := m.createPartitions(lastProcessedID, nextMaxID, partitionCount)

	// 保存分区到存储
	if err := m.savePartitionsToStorage(ctx, partitions); err != nil {
		return err
	}

	m.Logger.Infof("成功创建 %d 个分区，ID范围 [%d, %d]", partitionCount, lastProcessedID+1, nextMaxID)

	return nil
}

// getProcessingRange 获取需要处理的ID范围
func (m *Mgr) getProcessingRange(ctx context.Context) (int64, int64, error) {
	// 获取当前已处理的最大ID
	lastProcessedID, err := m.TaskProcessor.GetLastProcessedID(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "获取最后处理的ID失败")
	}

	// 获取下一批次的最大ID
	nextMaxID, err := m.TaskProcessor.GetNextMaxID(ctx, lastProcessedID, 10000) // 使用一个合理的范围大小
	if err != nil {
		return 0, 0, errors.Wrap(err, "获取下一个最大ID失败")
	}

	return lastProcessedID, nextMaxID, nil
}

// calculatePartitionCount 根据活跃工作节点计算分区数量
func (m *Mgr) calculatePartitionCount(ctx context.Context) (int, error) {
	// 获取活跃节点
	activeWorkers, err := m.getActiveWorkers(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "获取活跃节点失败")
	}

	// 计算分区数量
	partitionCount := len(activeWorkers)
	if partitionCount == 0 {
		partitionCount = 1 // 至少有一个分区
	} else if partitionCount > DefaultPartitionCount {
		partitionCount = DefaultPartitionCount // 限制最大分区数
	}

	return partitionCount, nil
}

// createPartitions 创建分区信息
func (m *Mgr) createPartitions(lastProcessedID, nextMaxID int64, partitionCount int) map[int]PartitionInfo {
	// 计算每个分区的ID范围
	partitionSize := (nextMaxID - lastProcessedID) / int64(partitionCount)
	if partitionSize < 1000 {
		partitionSize = 1000 // 确保每个分区至少有1000个ID
	}

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
			Status:      StatusPending,
			UpdatedAt:   time.Now(),
			Options:     make(map[string]interface{}),
		}
	}

	return partitions
}

// savePartitionsToStorage 保存分区信息到存储
func (m *Mgr) savePartitionsToStorage(ctx context.Context, partitions map[int]PartitionInfo) error {
	partitionInfoKey := fmt.Sprintf(PartitionInfoKeyFmt, m.Namespace)
	partitionsData, err := json.Marshal(partitions)
	if err != nil {
		return errors.Wrap(err, "序列化分区数据失败")
	}

	err = m.DataStore.SetPartitions(ctx, partitionInfoKey, string(partitionsData))
	if err != nil {
		return errors.Wrap(err, "保存分区数据失败")
	}

	return nil
}
