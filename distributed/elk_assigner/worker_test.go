package elk_assigner

import (
	"Lib/distributed/elk_assigner/data"
	"context"
	"encoding/json"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"sync"
	"testing"
	"time"
)

// 模拟 TaskProcessor 实现
type MockTaskProcessor struct {
	mock.Mock
}

func (m *MockTaskProcessor) Process(ctx context.Context, minID, maxID int64, opts map[string]interface{}) (int64, int64, error) {
	args := m.Called(ctx, minID, maxID, opts)
	return args.Get(0).(int64), args.Get(1).(int64), args.Error(2)
}

func (m *MockTaskProcessor) GetMaxProcessedID(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTaskProcessor) GetNextMaxID(ctx context.Context, currentMax int64, rangeSize int64) (int64, error) {
	args := m.Called(ctx, currentMax, rangeSize)
	return args.Get(0).(int64), args.Error(1)
}

// 可选实现，用于测试 updateGlobalMaxID
func (m *MockTaskProcessor) UpdateMaxProcessedID(ctx context.Context, newMaxID int64) error {
	args := m.Called(ctx, newMaxID)
	return args.Error(0)
}

// 模拟 Logger 实现
type MockLogger struct {
	mu     sync.Mutex
	Logs   map[string][]string
	silent bool
}

func NewMockLogger(silent bool) *MockLogger {
	return &MockLogger{
		Logs:   map[string][]string{"info": {}, "warn": {}, "error": {}, "debug": {}},
		silent: silent,
	}
}

func (l *MockLogger) Infof(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	message := formatLog(format, args...)
	l.Logs["info"] = append(l.Logs["info"], message)
}

func (l *MockLogger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	message := formatLog(format, args...)
	l.Logs["warn"] = append(l.Logs["warn"], message)
}

func (l *MockLogger) Errorf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	message := formatLog(format, args...)
	l.Logs["error"] = append(l.Logs["error"], message)
}

func (l *MockLogger) Debugf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	message := formatLog(format, args...)
	l.Logs["debug"] = append(l.Logs["debug"], message)
}

func formatLog(format string, args ...interface{}) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// 测试辅助函数，设置测试环境
func setupTest(t *testing.T) (*Worker, *MockTaskProcessor, *data.RedisDataStore, *miniredis.Miniredis, *MockLogger, func()) {
	// 启动 miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %v", err)
	}

	// 创建 Redis 客户端
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 创建 RedisDataStore
	opts := &data.Options{
		KeyPrefix:     "test:",
		DefaultExpiry: 5 * time.Second,
		MaxRetries:    3,
		RetryDelay:    10 * time.Millisecond,
		MaxRetryDelay: 50 * time.Millisecond,
	}
	dataStore := data.NewRedisDataStore(client, opts)

	// 创建 MockTaskProcessor
	processor := new(MockTaskProcessor)

	// 创建 MockLogger
	logger := NewMockLogger(true)

	// 创建 Worker
	worker := NewWorker("test_namespace", dataStore, processor)
	worker.SetLogger(logger)

	// 设置较短的超时时间以加速测试
	worker.SetTiming(
		500*time.Millisecond, // LeaderLockExpiry
		1*time.Second,        // PartitionLockExpiry
		100*time.Millisecond, // HeartbeatInterval
		200*time.Millisecond, // LeaderElectionInterval
		300*time.Millisecond, // ConsolidationInterval
	)

	// 清理函数
	cleanup := func() {
		worker.Stop()
		client.Close()
		mr.Close()
	}

	return worker, processor, dataStore, mr, logger, cleanup
}

func TestWorker_NewWorker(t *testing.T) {
	// 创建基本依赖
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	dataStore := data.NewRedisDataStore(client, data.DefaultOptions())
	processor := new(MockTaskProcessor)

	// 测试 worker 创建
	namespace := "test_worker"
	worker := NewWorker(namespace, dataStore, processor)

	// 验证默认值是否正确设置
	assert.NotEmpty(t, worker.ID)
	assert.Equal(t, namespace, worker.Namespace)
	assert.Equal(t, dataStore, worker.DataStore)
	assert.Equal(t, processor, worker.TaskProcessor)
	assert.NotNil(t, worker.Logger)
	assert.Equal(t, DefaultLeaderLockExpiry, worker.LeaderLockExpiry)
	assert.Equal(t, DefaultPartitionLockExpiry, worker.PartitionLockExpiry)
	assert.Equal(t, DefaultHeartbeatInterval, worker.HeartbeatInterval)
	assert.Equal(t, DefaultLeaderElectionInterval, worker.LeaderElectionInterval)
}

func TestWorker_SetTiming(t *testing.T) {
	worker, _, _, _, _, cleanup := setupTest(t)
	defer cleanup()

	// 设置自定义计时值
	leaderExpiry := 1 * time.Minute
	partitionExpiry := 2 * time.Minute
	heartbeat := 3 * time.Second
	leaderElection := 4 * time.Second
	consolidation := 5 * time.Minute

	worker.SetTiming(leaderExpiry, partitionExpiry, heartbeat, leaderElection, consolidation)

	// 验证值是否正确设置
	assert.Equal(t, leaderExpiry, worker.LeaderLockExpiry)
	assert.Equal(t, partitionExpiry, worker.PartitionLockExpiry)
	assert.Equal(t, heartbeat, worker.HeartbeatInterval)
	assert.Equal(t, leaderElection, worker.LeaderElectionInterval)
	assert.Equal(t, consolidation, worker.ConsolidationInterval)
}

func TestWorker_LeaderElection(t *testing.T) {
	// 创建共享的 miniredis 实例，确保 worker1 和 worker2 使用相同的 Redis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %v", err)
	}
	defer mr.Close()

	// 创建共享的 Redis 客户端和 DataStore
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	// 创建数据存储
	dataStore := data.NewRedisDataStore(client, &data.Options{
		KeyPrefix:     "test:",
		DefaultExpiry: 5 * time.Second,
	})

	// 统一的命名空间，确保两个 worker 使用相同的锁
	namespace := "test_leader_election"

	// 创建第一个 worker
	processor1 := new(MockTaskProcessor)
	worker1 := NewWorker(namespace, dataStore, processor1)
	worker1.SetTiming(
		500*time.Millisecond, // LeaderLockExpiry
		1*time.Second,        // PartitionLockExpiry
		100*time.Millisecond, // HeartbeatInterval
		200*time.Millisecond, // LeaderElectionInterval
		300*time.Millisecond, // ConsolidationInterval
	)
	logger1 := NewMockLogger(true)
	worker1.SetLogger(logger1)

	// 配置 processor1 返回预期值
	processor1.On("GetMaxProcessedID", mock.Anything).Return(int64(1000), nil)
	processor1.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(int64(2000), nil)
	processor1.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(int64(100), int64(1500), nil)
	processor1.On("UpdateMaxProcessedID", mock.Anything, mock.Anything).Return(nil)

	// 创建第二个 worker，使用相同的 dataStore
	processor2 := new(MockTaskProcessor)
	worker2 := NewWorker(namespace, dataStore, processor2)
	worker2.SetTiming(
		500*time.Millisecond, // LeaderLockExpiry
		1*time.Second,        // PartitionLockExpiry
		100*time.Millisecond, // HeartbeatInterval
		200*time.Millisecond, // LeaderElectionInterval
		300*time.Millisecond, // ConsolidationInterval
	)
	logger2 := NewMockLogger(true)
	worker2.SetLogger(logger2)

	// 配置 processor2 返回预期值
	processor2.On("GetMaxProcessedID", mock.Anything).Return(int64(1000), nil)
	processor2.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(int64(2000), nil)
	processor2.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(int64(100), int64(1500), nil)
	processor2.On("UpdateMaxProcessedID", mock.Anything, mock.Anything).Return(nil)

	// 使用足够长的超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 先验证当前没有 leader
	leaderKey := fmt.Sprintf("test:%s:leader_lock", namespace) // 直接使用完整键名
	initialValue, err := client.Get(ctx, leaderKey).Result()
	assert.Equal(t, redis.Nil, err, "初始状态应该没有 leader")
	assert.Empty(t, initialValue)

	// 启动第一个 worker，并等待它成为 leader
	go worker1.Start(ctx)
	time.Sleep(1 * time.Second) // 等待 worker1 成为 leader

	// 验证 worker1 成为了 leader
	isLeader1 := worker1.IsLeader()
	assert.True(t, isLeader1, "Worker1 应该成为 leader")

	// 直接从 Redis 验证 leader 锁
	leaderValue, err := client.Get(ctx, leaderKey).Result()
	assert.NoError(t, err, "应该能够获取 leader 锁")
	assert.Equal(t, worker1.ID, leaderValue, "Leader 锁应该包含 worker1 的 ID")

	// 启动第二个 worker
	go worker2.Start(ctx)
	time.Sleep(1 * time.Second) // 等待 worker2 尝试获取 leader 锁

	// worker2 不应该是 leader，因为 worker1 已经是 leader
	isLeader2 := worker2.IsLeader()
	assert.False(t, isLeader2, "Worker2 不应该是 leader，因为 worker1 已经是 leader")

	// 再次验证 leader 锁仍然是 worker1
	leaderValue, err = client.Get(ctx, leaderKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, worker1.ID, leaderValue, "Leader 锁应该仍然是 worker1 的 ID")

	// 停止 worker1，它应该释放 leader 锁
	worker1.Stop()

	// 等待足够长的时间确保 leader 锁过期
	time.Sleep(1 * time.Second)

	// 验证 leader 锁是否已过期或释放
	_, err = client.Get(ctx, leaderKey).Result()
	assert.Equal(t, redis.Nil, err, "Worker1 停止后，leader 锁应该已释放或过期")

	// 重新启动 worker2，它应该成为新的 leader
	// 如果之前的 worker2.Start 没有退出，这里先取消它
	worker2.Stop()
	time.Sleep(500 * time.Millisecond)

	go worker2.Start(ctx)
	time.Sleep(1 * time.Second) // 等待 worker2 成为 leader

	// 验证 worker2 是否成为新的 leader
	isLeader2 = worker2.IsLeader()
	assert.True(t, isLeader2, "Worker2 现在应该成为 leader")

	// 直接从 Redis 验证 leader 锁
	leaderValue, err = client.Get(ctx, leaderKey).Result()
	assert.NoError(t, err, "应该能够获取更新后的 leader 锁")
	assert.Equal(t, worker2.ID, leaderValue, "Leader 锁现在应该包含 worker2 的 ID")

	// 确保 worker2 被正确清理
	worker2.Stop()
}

func TestWorker_HeartbeatRegistration(t *testing.T) {
	worker, processor, dataStore, _, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// 为所有可能被调用的方法添加预期配置
	processor.On("GetMaxProcessedID", mock.Anything).Return(int64(1000), nil)
	processor.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(int64(2000), nil)
	processor.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(int64(100), int64(1500), nil)
	processor.On("UpdateMaxProcessedID", mock.Anything, mock.Anything).Return(nil)

	// 启动 worker
	go worker.Start(ctx)
	time.Sleep(300 * time.Millisecond)

	// 检查 worker 是否正确注册
	workers, err := dataStore.GetActiveWorkers(ctx, "test_namespace:workers")
	assert.NoError(t, err)
	assert.Contains(t, workers, worker.ID)

	// 验证心跳存在
	heartbeatKey := "test_namespace:heartbeat:" + worker.ID
	isActive, err := dataStore.IsWorkerActive(ctx, heartbeatKey)
	assert.NoError(t, err)
	assert.True(t, isActive)

	// 停止 worker
	worker.Stop()
	time.Sleep(200 * time.Millisecond)

	// 验证 worker 已注销
	workers, err = dataStore.GetActiveWorkers(ctx, "test_namespace:workers")
	assert.NoError(t, err)
	assert.NotContains(t, workers, worker.ID)

	// 验证心跳不再存在
	isActive, err = dataStore.IsWorkerActive(ctx, heartbeatKey)
	assert.NoError(t, err)
	assert.False(t, isActive)
}

func TestWorker_ProcessPartition(t *testing.T) {
	// 1. 创建独立的 Redis 测试实例
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %v", err)
	}
	defer mr.Close()

	// 2. 创建 Redis 客户端和 DataStore
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		_ = client.Close() // 处理关闭的错误
	}()

	// 3. 创建 DataStore 使用短期过期时间
	dataStore := data.NewRedisDataStore(client, &data.Options{
		KeyPrefix:     "test:",
		DefaultExpiry: 5 * time.Second,
	})

	// 4. 创建 Mock TaskProcessor 并设置预期
	processor := new(MockTaskProcessor)
	minID := int64(1000)
	maxID := int64(2000)
	processedCount := int64(500)
	finalMaxID := int64(1500)

	processor.On("GetMaxProcessedID", mock.Anything).Return(minID-100, nil)
	processor.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(maxID, nil)
	processor.On("Process", mock.Anything, minID, maxID, mock.MatchedBy(func(opts map[string]interface{}) bool {
		// 匹配任意选项
		return true
	})).Return(processedCount, finalMaxID, nil)
	processor.On("UpdateMaxProcessedID", mock.Anything, finalMaxID).Return(nil)

	// 添加通用匹配器以防万一
	processor.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(processedCount, finalMaxID, nil)

	// 5. 创建 Worker 实例
	namespace := "test_partition_processing"
	worker := NewWorker(namespace, dataStore, processor)
	worker.SetTiming(
		500*time.Millisecond, // LeaderLockExpiry
		1*time.Second,        // PartitionLockExpiry
		100*time.Millisecond, // HeartbeatInterval
		200*time.Millisecond, // LeaderElectionInterval
		300*time.Millisecond, // ConsolidationInterval
	)

	ctx := context.Background()

	// 6. 直接创建一个分区并保存到 Redis
	partitionInfo := PartitionInfo{
		PartitionID: 0,
		MinID:       minID,
		MaxID:       maxID,
		WorkerID:    "",
		Status:      "pending",
		UpdatedAt:   time.Now(),
		Options:     map[string]interface{}{},
	}

	partitions := map[int]PartitionInfo{0: partitionInfo}
	partitionsData, err := json.Marshal(partitions)
	assert.NoError(t, err)
	err = dataStore.SetPartitions(ctx, namespace+":partitions", string(partitionsData))
	assert.NoError(t, err)

	// 7. 手动获取分区锁
	// 使用这个变量
	acquired, err := dataStore.AcquireLock(ctx, namespace+":partition:0", worker.ID, worker.PartitionLockExpiry)
	assert.NoError(t, err)
	assert.True(t, acquired, "应该能够获取分区锁")

	// 8. 手动设置分区的 WorkerID（修复结构体赋值方式）
	updatedPartition := partitions[0]
	updatedPartition.WorkerID = worker.ID
	partitions[0] = updatedPartition

	updatedPartitionsData, err := json.Marshal(partitions)
	assert.NoError(t, err)
	err = dataStore.SetPartitions(ctx, namespace+":partitions", string(updatedPartitionsData))
	assert.NoError(t, err)

	// 更新 partitionInfo 以便传递给 processPartition 方法
	partitionInfo.WorkerID = worker.ID

	// 9. 直接调用 processPartition 方法
	err = worker.processPartition(ctx, &partitionInfo)
	assert.NoError(t, err, "processPartition 不应该返回错误")

	// 10. 验证结果
	// 获取更新后的分区信息
	partitionsStr, err := dataStore.GetPartitions(ctx, namespace+":partitions")
	assert.NoError(t, err)

	var retrievedPartitions map[int]PartitionInfo
	err = json.Unmarshal([]byte(partitionsStr), &retrievedPartitions)
	assert.NoError(t, err)

	// 检查分区状态是否已更新为 completed
	partition := retrievedPartitions[0]
	assert.Equal(t, "completed", partition.Status, "分区状态应该是 completed")
	assert.Equal(t, worker.ID, partition.WorkerID, "分区的 WorkerID 应该是 worker ID")

	// 验证 Process 方法被调用
	processor.AssertCalled(t, "Process", mock.Anything, minID, maxID, mock.Anything)

	// 验证 UpdateMaxProcessedID 方法被调用
	processor.AssertCalled(t, "UpdateMaxProcessedID", mock.Anything, finalMaxID)
}

// TestWorker_MultiWorkerPartitionHandling 测试多个 worker 协同处理分区的场景
func TestWorker_MultiWorkerPartitionHandling(t *testing.T) {
	// 创建 miniredis 实例用于测试
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %v", err)
	}
	defer mr.Close()

	// 创建 Redis 客户端
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	// 使用较短的时间间隔加快测试
	leaderExpiry := 1 * time.Second          // 减短领导锁过期时间
	partitionExpiry := 2 * time.Second       // 减短分区锁过期时间
	heartbeat := 200 * time.Millisecond      // 更快的心跳
	leaderElection := 500 * time.Millisecond // 更快的领导选举
	consolidation := 500 * time.Millisecond  // 更快的状态合并

	// 创建一个实际的任务处理器，而不是使用接口的 mock
	processor := &RealMockTaskProcessor{
		CurrentMaxID:      500000,                // 初始设置一个合理的最大ID
		ProcessDelay:      50 * time.Millisecond, // 减少处理延迟，加速测试
		mu:                sync.Mutex{},
		ProcessedRanges:   make(map[int64]struct{}),
		ProcessedByWorker: make(map[string][]int64), // 记录每个worker处理的分区
	}

	// 创建数据存储
	dataStore := data.NewRedisDataStore(client, &data.Options{
		KeyPrefix:     "test:",
		DefaultExpiry: 30 * time.Second,
	})

	// 创建三个 worker，使用相同的共享处理器
	workerCount := 3
	workers := make([]*Worker, workerCount)
	namespace := "test-namespace" // 使用相同的命名空间，让它们协同工作

	// 创建多个分区
	partitionCount := 10 // 创建10个分区
	partitions := make(map[int]PartitionInfo)

	// 为测试创建初始分区数据
	for i := 0; i < partitionCount; i++ {
		minID := int64(500000 + i*10000)
		maxID := int64(500000 + (i+1)*10000)
		partitions[i] = PartitionInfo{
			PartitionID: i,
			MinID:       minID,
			MaxID:       maxID,
			WorkerID:    "",
			Status:      "pending",
			UpdatedAt:   time.Now(),
			Options:     map[string]interface{}{},
		}
	}

	// 保存分区数据到Redis
	partitionsData, err := json.Marshal(partitions)
	assert.NoError(t, err)
	err = dataStore.SetPartitions(context.Background(), "test:"+namespace+":partitions", string(partitionsData))
	assert.NoError(t, err)

	// 创建所有worker实例
	for i := 0; i < workerCount; i++ {
		workers[i] = NewWorker(namespace, dataStore, processor)
		workers[i].SetTiming(leaderExpiry, partitionExpiry, heartbeat, leaderElection, consolidation)
	}

	// 启动所有 worker
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i, worker := range workers {
		wg.Add(1)
		go func(w *Worker, idx int) {
			defer wg.Done()
			if err := w.Start(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				t.Errorf("Worker %d 发生错误: %v", idx, err)
			}
		}(worker, i)
	}

	// 给worker足够的时间来处理分区
	time.Sleep(10 * time.Second)

	// 停止所有worker
	for _, worker := range workers {
		worker.Stop()
	}

	wg.Wait()

	// 从Redis获取最终分区状态
	partitionsStr, err := dataStore.GetPartitions(ctx, "test:"+namespace+":partitions")
	assert.NoError(t, err)

	var finalPartitions map[int]PartitionInfo
	err = json.Unmarshal([]byte(partitionsStr), &finalPartitions)
	assert.NoError(t, err)

	// 检查处理器状态
	processor.mu.Lock()
	maxID := processor.CurrentMaxID
	processedRanges := len(processor.ProcessedRanges)
	workerProcessed := processor.ProcessedByWorker
	processor.mu.Unlock()

	t.Logf("最终全局最大ID: %d, 已处理区间数: %d", maxID, processedRanges)

	// 验证所有分区都已被处理
	completedPartitions := 0
	for _, partition := range finalPartitions {
		if partition.Status == "completed" {
			completedPartitions++
		}
	}

	// 打印各个worker处理的分区情况
	for workerID, partitions := range workerProcessed {
		t.Logf("Worker %s 处理了 %d 个分区: %v", workerID, len(partitions), partitions)
	}

	// 验证：
	// 1. 所有分区都应该被处理完成
	assert.Equal(t, partitionCount, completedPartitions, "所有分区应该都被处理完成")

	// 2. 最大ID应该大于初始值
	assert.Greater(t, maxID, int64(500000), "处理后的最大ID应该大于初始值")

	// 3. 处理的分区数应该等于总分区数
	assert.Equal(t, partitionCount, processedRanges, "处理的分区数应等于总分区数")

	// 4. 验证没有分区被多个worker重复处理
	processedPartitions := make(map[int64]string) // 分区ID -> workerID
	duplicateProcessed := false

	for workerID, partitions := range workerProcessed {
		for _, partID := range partitions {
			if prevWorker, exists := processedPartitions[partID]; exists {
				t.Errorf("分区 %d 被多个worker处理: %s 和 %s", partID, prevWorker, workerID)
				duplicateProcessed = true
			}
			processedPartitions[partID] = workerID
		}
	}

	assert.False(t, duplicateProcessed, "不应该有分区被多个worker重复处理")
}

// 为多worker测试定义的实际任务处理器
type RealMockTaskProcessor struct {
	CurrentMaxID      int64
	ProcessDelay      time.Duration
	mu                sync.Mutex
	ProcessedRanges   map[int64]struct{} // 记录已处理的区间起点
	ProcessedByWorker map[string][]int64 // 记录每个worker处理的分区
	FailedPartition   int                // 可以设置一个特定分区失败用于测试
}

func (m *RealMockTaskProcessor) Process(ctx context.Context, minID, maxID int64, opts map[string]interface{}) (int64, int64, error) {
	// 特殊处理：更新最大ID的请求
	if action, ok := opts["action"].(string); ok && action == "update_max_id" {
		if newMaxID, ok := opts["new_max_id"].(int64); ok {
			m.mu.Lock()
			if newMaxID > m.CurrentMaxID {
				m.CurrentMaxID = newMaxID
			}
			m.mu.Unlock()
			return 0, newMaxID, nil
		}
	}

	// 模拟处理延迟
	select {
	case <-ctx.Done():
		return 0, minID, ctx.Err()
	case <-time.After(m.ProcessDelay):
		// 继续处理
	}

	// 获取分区ID（用于失败测试）
	partitionID := -1
	if partOpt, ok := opts["partition_id"].(int); ok {
		partitionID = partOpt
	}

	// 对于测试，如果这个分区ID等于设置的失败分区，则返回错误
	if m.FailedPartition > 0 && partitionID == m.FailedPartition {
		return 0, minID, fmt.Errorf("模拟分区 %d 处理失败", partitionID)
	}

	// 更新已处理区间记录和状态
	m.mu.Lock()
	defer m.mu.Unlock()

	// 记录处理过的区间
	m.ProcessedRanges[minID] = struct{}{}

	// 记录处理的分区到对应的worker
	if workerID, ok := opts["worker_id"].(string); ok {
		m.ProcessedByWorker[workerID] = append(m.ProcessedByWorker[workerID], minID)
	}

	// 模拟处理项目
	processCount := (maxID - minID)
	if processCount < 0 {
		processCount = 0
	}

	// 更新 CurrentMaxID 如果处理��项目
	if maxID > m.CurrentMaxID {
		m.CurrentMaxID = maxID
	}

	// 返回处理结果
	return processCount, maxID, nil
}

func (m *RealMockTaskProcessor) GetMaxProcessedID(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CurrentMaxID, nil
}

func (m *RealMockTaskProcessor) GetNextMaxID(ctx context.Context, currentMax int64, rangeSize int64) (int64, error) {
	return currentMax + rangeSize, nil
}

func (m *RealMockTaskProcessor) UpdateMaxProcessedID(ctx context.Context, newMaxID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if newMaxID > m.CurrentMaxID {
		m.CurrentMaxID = newMaxID
	}
	return nil
}

func TestWorker_ConsolidateGlobalState(t *testing.T) {
	worker, processor, dataStore, _, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// 设置处理器行为
	currentMaxID := int64(1000)
	processor.On("GetMaxProcessedID", mock.Anything).Return(currentMaxID, nil)
	processor.On("GetNextMaxID", mock.Anything, currentMaxID, mock.Anything).Return(int64(4000), nil)
	processor.On("UpdateMaxProcessedID", mock.Anything, int64(3000)).Return(nil)

	// 创建已完成的分区数据
	partitions := map[int]PartitionInfo{
		0: {
			PartitionID: 0,
			MinID:       1001, // 从 currentMaxID+1 开始
			MaxID:       2000,
			WorkerID:    "worker1",
			Status:      "completed",
			UpdatedAt:   time.Now(),
		},
		1: {
			PartitionID: 1,
			MinID:       2001,
			MaxID:       3000,
			WorkerID:    "worker2",
			Status:      "completed",
			UpdatedAt:   time.Now(),
		},
		2: {
			PartitionID: 2,
			MinID:       3001,
			MaxID:       4000,
			WorkerID:    "worker3",
			Status:      "running", // 这个分区还在运行中
			UpdatedAt:   time.Now(),
		},
	}

	// 保存分区数据到 Redis
	partitionsData, err := json.Marshal(partitions)
	assert.NoError(t, err)
	err = dataStore.SetPartitions(ctx, "test_namespace:partitions", string(partitionsData))
	assert.NoError(t, err)

	// 启动 worker，让它成为 leader
	go worker.Start(ctx)
	time.Sleep(1 * time.Second)

	// 验证 consolidateMaxID 是否被调用
	processor.AssertCalled(t, "UpdateMaxProcessedID", mock.Anything, int64(3000))
}

func TestWorker_FailureRecovery(t *testing.T) {
	// 创建共享的 miniredis 实例
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %v", err)
	}
	defer mr.Close()

	// 创建共享的 Redis 客户端和 DataStore
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	dataStore := data.NewRedisDataStore(client, data.DefaultOptions())

	// 设置初始状态：一个失败的分区
	ctx := context.Background()
	partitions := map[int]PartitionInfo{
		0: {
			PartitionID: 0,
			MinID:       1000,
			MaxID:       2000,
			WorkerID:    "failed-worker",
			Status:      "failed",
			UpdatedAt:   time.Now().Add(-10 * time.Minute), // 很久以前失败的
		},
	}

	partitionsData, err := json.Marshal(partitions)
	assert.NoError(t, err)
	err = dataStore.SetPartitions(ctx, "recovery_test:partitions", string(partitionsData))
	assert.NoError(t, err)

	// 创建新的 worker
	processor := new(MockTaskProcessor)
	processor.On("GetMaxProcessedID", mock.Anything).Return(int64(500), nil) // 之前处理了 500
	processor.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(int64(3000), nil)
	processor.On("Process", mock.Anything, int64(1000), int64(2000), mock.Anything).
		Return(int64(1000), int64(2000), nil)
	processor.On("UpdateMaxProcessedID", mock.Anything, int64(2000)).Return(nil)

	worker := NewWorker("recovery_test", dataStore, processor)
	worker.SetTiming(
		500*time.Millisecond, // LeaderLockExpiry
		1*time.Second,        // PartitionLockExpiry
		100*time.Millisecond, // HeartbeatInterval
		200*time.Millisecond, // LeaderElectionInterval
		300*time.Millisecond, // ConsolidationInterval
	)
	logger := NewMockLogger(true)
	worker.SetLogger(logger)

	// 启动 worker
	go worker.Start(ctx)
	time.Sleep(2 * time.Second)

	// 验证分区是否被重新处理
	partitionsStr, err := dataStore.GetPartitions(ctx, "recovery_test:partitions")
	assert.NoError(t, err)

	var retrievedPartitions map[int]PartitionInfo
	err = json.Unmarshal([]byte(partitionsStr), &retrievedPartitions)
	assert.NoError(t, err)

	// 验证分区状态是否更新
	assert.Equal(t, "completed", retrievedPartitions[0].Status)
	assert.Equal(t, worker.ID, retrievedPartitions[0].WorkerID)

	// 验证处理器方法是否被调用
	processor.AssertCalled(t, "Process", mock.Anything, int64(1000), int64(2000), mock.Anything)
	processor.AssertCalled(t, "UpdateMaxProcessedID", mock.Anything, int64(2000))

	// 清理
	worker.Stop()
}

func TestWorker_PartitionLockExpiry(t *testing.T) {
	worker1, processor1, dataStore, mr, _, cleanup1 := setupTest(t)
	defer cleanup1()

	ctx := context.Background()

	// 设置处理器行为 - worker1 的处理器会阻塞
	processor1.On("GetMaxProcessedID", mock.Anything).Return(int64(1000), nil)
	processor1.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(int64(3000), nil)

	// 设置一个永久阻塞的处理方法
	blockCh := make(chan struct{})
	processor1.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 永久阻塞
			<-blockCh
		}).Return(int64(0), int64(0), nil)

	// 创建测试分区数据
	partitions := map[int]PartitionInfo{
		0: {
			PartitionID: 0,
			MinID:       1000,
			MaxID:       2000,
			WorkerID:    "",
			Status:      "pending",
			UpdatedAt:   time.Now(),
		},
	}

	partitionsData, err := json.Marshal(partitions)
	assert.NoError(t, err)
	err = dataStore.SetPartitions(ctx, "test_namespace:partitions", string(partitionsData))
	assert.NoError(t, err)

	// 启动第一个 worker
	go worker1.Start(ctx)
	time.Sleep(500 * time.Millisecond) // 给 worker1 足够时间获取锁

	// 创建第二个 worker
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	processor2 := new(MockTaskProcessor)
	processor2.On("GetMaxProcessedID", mock.Anything).Return(int64(1000), nil)
	processor2.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(int64(3000), nil)
	processor2.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(int64(1000), int64(2000), nil)
	processor2.On("UpdateMaxProcessedID", mock.Anything, int64(2000)).Return(nil)

	worker2 := NewWorker("test_namespace", dataStore, processor2)
	worker2.SetTiming(
		500*time.Millisecond, // LeaderLockExpiry
		1*time.Second,        // PartitionLockExpiry - 设置短的过期时间
		100*time.Millisecond, // HeartbeatInterval
		200*time.Millisecond, // LeaderElectionInterval
		300*time.Millisecond, // ConsolidationInterval
	)
	logger2 := NewMockLogger(true)
	worker2.SetLogger(logger2)

	// 停止 worker1 而不释放锁
	worker1.cancelWork() // 只取消工作上下文，但不释放锁

	// 等待锁过期
	time.Sleep(1500 * time.Millisecond)

	// 启动第二个 worker
	go worker2.Start(ctx)
	time.Sleep(1 * time.Second)

	// 验证第二个 worker 是否重新处理了分区
	partitionsStr, err := dataStore.GetPartitions(ctx, "test_namespace:partitions")
	assert.NoError(t, err)

	var retrievedPartitions map[int]PartitionInfo
	err = json.Unmarshal([]byte(partitionsStr), &retrievedPartitions)
	assert.NoError(t, err)

	// 验证分区状态是否更新为已完成
	assert.Equal(t, "completed", retrievedPartitions[0].Status)
	assert.Equal(t, worker2.ID, retrievedPartitions[0].WorkerID)

	// 验证第二个处理器方法是否被调用
	processor2.AssertCalled(t, "Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	// 清理
	close(blockCh)
	worker2.Stop()
	client.Close()
}
