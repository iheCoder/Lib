package elk_assigner

import (
	"Lib/distributed/elk_assigner/data"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"os"
	"sync"
	"time"
)

// Mgr 是一个分布式管理器，管理分区任务的执行
type Mgr struct {
	ID            string
	Namespace     string
	DataStore     data.DataStore
	TaskProcessor Processor
	Logger        Logger

	// 配置选项
	HeartbeatInterval      time.Duration
	LeaderElectionInterval time.Duration
	PartitionLockExpiry    time.Duration
	LeaderLockExpiry       time.Duration

	// 内部状态
	isLeader        bool
	heartbeatCtx    context.Context
	cancelHeartbeat context.CancelFunc
	leaderCtx       context.Context
	cancelLeader    context.CancelFunc
	workCtx         context.Context
	cancelWork      context.CancelFunc

	mu sync.RWMutex
}

// NewMgr 创建一个新的管理器实例
func NewMgr(namespace string, dataStore data.DataStore, processor Processor) *Mgr {
	nodeID := generateNodeID()
	logger := &defaultLogger{}

	return &Mgr{
		ID:                     nodeID,
		Namespace:              namespace,
		DataStore:              dataStore,
		TaskProcessor:          processor,
		Logger:                 logger,
		HeartbeatInterval:      DefaultHeartbeatInterval,
		LeaderElectionInterval: DefaultLeaderElectionInterval,
		PartitionLockExpiry:    DefaultPartitionLockExpiry,
		LeaderLockExpiry:       DefaultLeaderLockExpiry,
	}
}

// SetLogger 设置自定义日志记录器
func (m *Mgr) SetLogger(logger Logger) {
	m.Logger = logger
}

// generateNodeID 生成唯一的节点ID
func generateNodeID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s-%d-%s", hostname, os.Getpid(), uuid.New().String()[:8])
}

// Start 启动管理器
func (m *Mgr) Start(ctx context.Context) error {
	m.Logger.Infof("启动管理器 %s, 命名空间: %s", m.ID, m.Namespace)

	// 创建上下文
	m.heartbeatCtx, m.cancelHeartbeat = context.WithCancel(context.Background())
	m.workCtx, m.cancelWork = context.WithCancel(context.Background())

	// 注册本节点，并周期发送心跳
	go m.nodeKeeper(ctx)

	// 做leader相关的工作
	go m.Lead(ctx)

	// 处理分配任务
	go m.Handle(ctx)

	return nil
}

// Stop 停止管理器
func (m *Mgr) Stop() {
	m.Logger.Infof("停止管理器 %s", m.ID)

	// 取消所有运行中的任务
	if m.cancelHeartbeat != nil {
		m.cancelHeartbeat()
	}

	if m.cancelLeader != nil {
		m.cancelLeader()
	}

	if m.cancelWork != nil {
		m.cancelWork()
	}

	// 释放锁和清理
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m.mu.RLock()
	isLeader := m.isLeader
	m.mu.RUnlock()

	if isLeader {
		leaderLockKey := fmt.Sprintf(LeaderLockKeyFmt, m.Namespace)
		err := m.DataStore.ReleaseLock(ctx, leaderLockKey, m.ID)
		if err != nil {
			m.Logger.Warnf("释放Leader锁失败: %v", err)
		}
	}

	// 删除心跳
	heartbeatKey := fmt.Sprintf(HeartbeatFmtFmt, m.Namespace, m.ID)
	m.DataStore.DeleteKey(ctx, heartbeatKey)

	// 注销节点
	workersKey := fmt.Sprintf(WorkersKeyFmt, m.Namespace)
	m.DataStore.UnregisterWorker(ctx, workersKey, m.ID, heartbeatKey)

	m.Logger.Infof("管理器 %s 已停止", m.ID)
}

// IsLeader 返回当前节点是否是Leader
func (m *Mgr) IsLeader() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isLeader
}

// nodeKeeper 管理节点注册和心跳
func (m *Mgr) nodeKeeper(ctx context.Context) {
	m.Logger.Infof("开始节点维护任务")

	// 注册本节点
	err := m.registerNode(ctx)
	if err != nil {
		m.Logger.Errorf("注册节点失败: %v", err)
		return
	}

	// 周期性发送心跳
	ticker := time.NewTicker(m.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.Logger.Infof("节点维护任务停止 (上下文取消)")
			return
		case <-m.heartbeatCtx.Done():
			m.Logger.Infof("节点维护任务停止 (心跳上下文取消)")
			return
		case <-ticker.C:
			heartbeatKey := fmt.Sprintf(HeartbeatFmtFmt, m.Namespace, m.ID)
			err := m.DataStore.SetHeartbeat(ctx, heartbeatKey, time.Now().Format(time.RFC3339))
			if err != nil {
				m.Logger.Warnf("发送心跳失败: %v", err)
			}
		}
	}
}

// registerNode 注册节点到系统
func (m *Mgr) registerNode(ctx context.Context) error {
	heartbeatKey := fmt.Sprintf(HeartbeatFmtFmt, m.Namespace, m.ID)
	workersKey := fmt.Sprintf(WorkersKeyFmt, m.Namespace)

	err := m.DataStore.RegisterWorker(
		ctx,
		workersKey,
		m.ID,
		heartbeatKey,
		time.Now().Format(time.RFC3339),
	)

	if err != nil {
		return errors.Wrap(err, "注册节点失败")
	}

	m.Logger.Infof("节点 %s 已注册", m.ID)
	return nil
}

// getActiveWorkers 获取活跃节点列表
func (m *Mgr) getActiveWorkers(ctx context.Context) ([]string, error) {
	pattern := fmt.Sprintf(HeartbeatFmtFmt, m.Namespace, "*")
	keys, err := m.DataStore.GetKeys(ctx, pattern)
	if err != nil {
		return nil, errors.Wrap(err, "获取心跳键失败")
	}

	var activeWorkers []string
	now := time.Now()
	validHeartbeatDuration := m.HeartbeatInterval * 3

	for _, key := range keys {
		// 从key中提取节点ID
		prefix := fmt.Sprintf(HeartbeatFmtFmt, m.Namespace, "")
		nodeID := key[len(prefix):]

		// 获取最后心跳时间
		lastHeartbeatStr, err := m.DataStore.GetHeartbeat(ctx, key)
		if err != nil {
			continue // 跳过错误的心跳
		}

		lastHeartbeat, err := time.Parse(time.RFC3339, lastHeartbeatStr)
		if err != nil {
			continue // 跳过无效的时间格式
		}

		// 检查心跳是否有效
		if now.Sub(lastHeartbeat) <= validHeartbeatDuration {
			activeWorkers = append(activeWorkers, nodeID)
		} else {
			// 删除过期心跳
			m.DataStore.DeleteKey(ctx, key)
		}
	}

	return activeWorkers, nil
}
