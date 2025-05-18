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

func (m *MockTaskProcessor) Process(ctx context.Context, minID, maxID int64, opts map[string]interface{}) (int64, error) {
	args := m.Called(ctx, minID, maxID, opts)
	return args.Get(0).(int64), args.Error(1)
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
		Return(int64(100), nil)
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
		Return(int64(100), nil)
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
	processor.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(int64(100), nil)
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
	processedCount := int64(1000)

	processor.On("GetMaxProcessedID", mock.Anything).Return(minID, nil)
	processor.On("GetNextMaxID", mock.Anything, mock.Anything, mock.Anything).Return(maxID, nil)
	processor.On("Process", mock.Anything, minID, maxID, mock.MatchedBy(func(opts map[string]interface{}) bool {
		// 匹配任意选项
		return true
	})).Return(processedCount, nil)
	processor.On("UpdateMaxProcessedID", mock.Anything, maxID).Return(nil)

	// 添加通用匹配器以防万一
	processor.On("Process", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(processedCount, nil)

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
	processor.AssertCalled(t, "UpdateMaxProcessedID", mock.Anything, maxID)
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
	totalData := 150_000
	initialMaxID := 0
	processor := NewRealMockTaskProcessor(int64(initialMaxID), int64(totalData), int64(initialMaxID+totalData))

	// 创建数据存储
	dataStore := data.NewRedisDataStore(client, &data.Options{
		KeyPrefix:     "test:",
		DefaultExpiry: 30 * time.Second,
	})

	// 创建三个 worker，使用相同的共享处理器
	workerCount := 3
	workers := make([]*Worker, workerCount)
	namespace := "test-namespace" // 使用相同的命名空间，让它们协同工作

	// 创建所有worker实例
	for i := 0; i < workerCount; i++ {
		workers[i] = NewWorker(namespace, dataStore, processor)
		workers[i].SetTiming(leaderExpiry, partitionExpiry, heartbeat, leaderElection, consolidation)
	}

	// 启动所有 worker
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i, worker := range workers {
		wg.Add(1)
		go func(w *Worker, idx int) {
			defer wg.Done()
			if err := w.Start(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				t.Errorf("Worker %d 发生错误: %v", idx, err)
			}
			t.Logf("Worker %d 启动完成", idx)
		}(worker, i)
	}

	// 等待数据处理完成或超时
	complete := make(chan bool)
	go func() {
		checkInterval := 500 * time.Millisecond
		maxWait := 15 * time.Second
		waited := 0 * time.Second

		for waited < maxWait {
			// 检查是否所有数据都已处理完成
			if processor.IsAllDataProcessed() {
				complete <- true
				return
			}

			time.Sleep(checkInterval)
			waited += checkInterval
		}

		// 超时
		complete <- false
	}()

	// 等待处理完成或超时
	success := <-complete

	// 停止所有worker
	for _, worker := range workers {
		worker.Stop()
	}

	// 等待所有worker协同退出
	wg.Wait()

	// 增加调试信息：列出所有Redis键
	keys, err := client.Keys(ctx, "test:*").Result()
	if err != nil {
		t.Logf("获取Redis键失败: %v", err)
	} else {
		t.Logf("Redis中的键: %v", keys)
	}

	// 正确构建分区键名（使用worker实例中的方法获取）
	partitionsKey := "test:" + namespace + ":partitions"
	t.Logf("尝试获取分区数据，使用键: %s", partitionsKey)

	// 从Redis获取最终分区状态
	partitionsStr, err := client.Get(ctx, partitionsKey).Result()
	if err != nil {
		t.Logf("获取分区数据失败: %v, 键: %s", err, partitionsKey)
		// 不要中止测试，继续进行其他检查
	}

	t.Logf("获取到的分区数据长度: %d", len(partitionsStr))

	var finalPartitions map[int]PartitionInfo
	if partitionsStr != "" {
		err = json.Unmarshal([]byte(partitionsStr), &finalPartitions)
		assert.NoError(t, err)
	} else {
		t.Logf("分区数据为空，创建空映射")
		finalPartitions = make(map[int]PartitionInfo)
	}

	// 检查处理器状态
	processor.mu.Lock()
	maxID := processor.CurrentMaxID
	processedRanges := len(processor.ProcessedRanges)
	workerProcessed := processor.ProcessedByWorker
	processor.mu.Unlock()

	// 现��单独调用，避免死锁
	allDataProcessed := processor.IsAllDataProcessed()

	// 打印处理状态信息
	t.Logf("最终全局最大ID: %d, 已处理区间数: %d, 所有数据已处理: %v",
		maxID, processedRanges, allDataProcessed)

	// 验证所有分区都已被处理
	completedPartitions := 0
	if len(finalPartitions) > 0 {
		for partID, partition := range finalPartitions {
			t.Logf("分区 %d 状态: %s, 范围: [%d, %d], 工作节点: %s",
				partID, partition.Status, partition.MinID, partition.MaxID, partition.WorkerID)

			if partition.Status == "completed" {
				completedPartitions++
			}
		}
	}

	// 打印各个worker处理的分区情况
	for workerID, partitions := range workerProcessed {
		t.Logf("Worker %s 处理了 %d 个分区: %v", workerID, len(partitions), partitions)
	}

	// 验证：
	// 1. 应该有处理完成的数据
	assert.Greater(t, processedRanges, 0, "应该有数据被处理")

	// 2. 最大ID应该大于初始值
	assert.Greater(t, maxID, int64(500000), "处理后的最大ID应该大于初始值")

	// 仅在处理成功时验证这些条件
	if success {
		// 3. 验证没有分区被多个worker重复处理
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
		Return(int64(1000), nil)
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
		}).Return(int64(0), nil)

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
		Return(int64(1000), nil)
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

// RealMockTaskProcessor 改进，添加完成标志和最大ID限制
type RealMockTaskProcessor struct {
	CurrentMaxID      int64
	ProcessDelay      time.Duration
	mu                sync.Mutex
	ProcessedRanges   map[int64]struct{} // 记录已处理的区间起点
	ProcessedByWorker map[string][]int64 // 记录每个worker处理的分区
	FailedPartition   int                // 可以设置一个特定分区失败用于测试
	TotalDataSize     int64              // 总数据量
	ProcessedDataSize int64              // 已处理的数据量
	MaxPossibleID     int64              // 最大可能的ID值，超过此值表示处理完成
	IsTerminated      bool               // 标记处理器是否已终止
	FinishChan        chan struct{}      // 通知处理完成的通道
}

func NewRealMockTaskProcessor(initialMaxID int64, totalData int64, maxPossibleID int64) *RealMockTaskProcessor {
	return &RealMockTaskProcessor{
		CurrentMaxID:      initialMaxID,
		ProcessDelay:      50 * time.Millisecond,
		ProcessedRanges:   make(map[int64]struct{}),
		ProcessedByWorker: make(map[string][]int64),
		TotalDataSize:     totalData,
		MaxPossibleID:     maxPossibleID,
		IsTerminated:      false,
		FinishChan:        make(chan struct{}),
	}
}

func (m *RealMockTaskProcessor) Process(ctx context.Context, minID, maxID int64, opts map[string]interface{}) (int64, error) {
	// 特殊处理：更新最大ID的请求
	if action, ok := opts["action"].(string); ok && action == "update_max_id" {
		if newMaxID, ok := opts["new_max_id"].(int64); ok {
			m.mu.Lock()
			if newMaxID > m.CurrentMaxID {
				m.CurrentMaxID = newMaxID
			}
			m.mu.Unlock()
			return 0, nil
		}
	}

	// 检查是否已终止
	m.mu.Lock()
	if m.IsTerminated || m.CurrentMaxID >= m.MaxPossibleID {
		m.mu.Unlock()
		return 0, fmt.Errorf("processor已终止")
	}
	m.mu.Unlock()

	// 模拟处理延迟
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(m.ProcessDelay):
		// 继续处理
	}

	// 获取分区ID和WorkerID
	partitionID := -1
	workerID := ""

	if partOpt, ok := opts["partition_id"].(int); ok {
		partitionID = partOpt
	}

	if wID, ok := opts["worker_id"].(string); ok {
		workerID = wID
	}

	// 对于测试，如果这个分区ID等于设置的失败分区，则返回错误
	if m.FailedPartition > 0 && partitionID == m.FailedPartition {
		return 0, fmt.Errorf("模拟分区 %d 处理失败", partitionID)
	}

	// 更新已处理区间记录和状态
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已达到最大ID
	if m.CurrentMaxID >= m.MaxPossibleID {
		if !m.IsTerminated {
			m.IsTerminated = true
			close(m.FinishChan) // 通知处理完成
		}
		return 0, fmt.Errorf("已达到最大ID %d，处理终止", m.MaxPossibleID)
	}

	// 记录处理过的区间
	m.ProcessedRanges[minID] = struct{}{}

	// 记录处理的分区到对应的worker
	if workerID != "" {
		m.ProcessedByWorker[workerID] = append(m.ProcessedByWorker[workerID], minID)
	}

	// 模拟处理项目
	processCount := maxID - minID
	if processCount < 0 {
		processCount = 0
	}

	// 累计已处理数据量
	m.ProcessedDataSize += processCount

	// 更新 CurrentMaxID 如果处理了项目
	if maxID > m.CurrentMaxID {
		m.CurrentMaxID = maxID
	}

	// 检查是否已完成所有数据处理
	if m.TotalDataSize > 0 && m.ProcessedDataSize >= m.TotalDataSize ||
		m.CurrentMaxID >= m.MaxPossibleID {
		if !m.IsTerminated {
			m.IsTerminated = true
			close(m.FinishChan) // 通知处理完成
		}
	}

	// 返回处理结果
	return processCount, nil
}

func (m *RealMockTaskProcessor) GetMaxProcessedID(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CurrentMaxID, nil
}

func (m *RealMockTaskProcessor) GetNextMaxID(ctx context.Context, currentMax int64, rangeSize int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已终止，返回当前最大ID，不再增长
	if m.IsTerminated || m.CurrentMaxID >= m.MaxPossibleID {
		return m.CurrentMaxID, nil
	}

	// 如果设置了最大可能ID，不要超过它
	if m.MaxPossibleID > 0 && currentMax+rangeSize > m.MaxPossibleID {
		return m.MaxPossibleID, nil
	}

	// 如果��置了总数据大小，并且我们已经处理了足够多的数据
	if m.TotalDataSize > 0 && m.ProcessedDataSize >= m.TotalDataSize {
		return m.CurrentMaxID, nil
	}

	return currentMax + rangeSize, nil
}

func (m *RealMockTaskProcessor) UpdateMaxProcessedID(ctx context.Context, newMaxID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.IsTerminated {
		return nil
	}

	if newMaxID > m.CurrentMaxID {
		m.CurrentMaxID = newMaxID
	}

	if m.CurrentMaxID >= m.MaxPossibleID {
		if !m.IsTerminated {
			m.IsTerminated = true
			close(m.FinishChan) // 通知处理完成
		}
	}

	return nil
}

// IsAllDataProcessed 检查是否所有数据都已处理完成
func (m *RealMockTaskProcessor) IsAllDataProcessed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.IsTerminated || m.CurrentMaxID >= m.MaxPossibleID {
		return true
	}

	if m.TotalDataSize > 0 {
		return m.ProcessedDataSize >= m.TotalDataSize
	}

	return false // 无法判断是否完成
}

// WaitForCompletion 等待处理完成
func (m *RealMockTaskProcessor) WaitForCompletion(timeout time.Duration) bool {
	select {
	case <-m.FinishChan:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestWorker_IsSafeToUpdateGlobalMaxID(t *testing.T) {
	worker, processor, dataStore, _, _, cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()

	// 为处理器设置必要的行为
	processor.On("GetMaxProcessedID", mock.Anything).Return(int64(1000), nil)

	// 测试场景1: 直接跟随全局最大ID的分区 - 应该返回安全
	t.Run("PartitionDirectlyFollowingGlobalMaxID", func(t *testing.T) {
		// 准备分区数据
		partitions := map[int]PartitionInfo{
			0: {
				PartitionID: 0,
				MinID:       100,
				MaxID:       1000, // 全局最大ID是1000
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
			1: { // 这个分区直接跟随全局最大ID
				PartitionID: 1,
				MinID:       1001, // 从GlobalMaxID+1开始
				MaxID:       2000,
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
			2: {
				PartitionID: 2,
				MinID:       2001,
				MaxID:       3000,
				Status:      "pending", // 未完成
				UpdatedAt:   time.Now(),
			},
		}

		// 保存分区数据
		partitionsData, err := json.Marshal(partitions)
		assert.NoError(t, err)
		err = dataStore.SetPartitions(ctx, worker.getPartitionInfoKey(), string(partitionsData))
		assert.NoError(t, err)

		// 测试是否安全更新
		safe, err := worker.isSafeToUpdateGlobalMaxID(ctx, 1, 2000) // 分区ID 1, 新最大ID 2000
		assert.NoError(t, err)
		assert.True(t, safe, "直接跟随全局最大ID的分区应该安全更新")
	})

	// 测试场景2: 中间有未完成分区 - 应该返回不安全
	t.Run("PartitionWithPendingInBetween", func(t *testing.T) {
		// 准备分区数据
		partitions := map[int]PartitionInfo{
			0: {
				PartitionID: 0,
				MinID:       100,
				MaxID:       500,
				Status:      "pending", // 未完成的分区
				UpdatedAt:   time.Now(),
			},
			1: {
				PartitionID: 1,
				MinID:       500,
				MaxID:       1500,
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
		}

		// 保存分区数据
		partitionsData, err := json.Marshal(partitions)
		assert.NoError(t, err)
		err = dataStore.SetPartitions(ctx, worker.getPartitionInfoKey(), string(partitionsData))
		assert.NoError(t, err)

		// 测试是否安全更新
		safe, err := worker.isSafeToUpdateGlobalMaxID(ctx, 1, 1500) // 分区ID 1, 新最大ID 1500
		assert.NoError(t, err)
		assert.False(t, safe, "有未完成分区时不应该安全更新")
	})

	// 测试场景3: 有间隙但由已完成分区覆盖 - 应该返回安全
	t.Run("GapCoveredByCompletedPartition", func(t *testing.T) {
		// 准备分区数据
		partitions := map[int]PartitionInfo{
			0: {
				PartitionID: 0,
				MinID:       100,
				MaxID:       1000, // 全局最大ID
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
			1: { // 覆盖间隙的分区
				PartitionID: 1,
				MinID:       500,  // 小于全局最大ID
				MaxID:       1500, // 大于下一分区的MinID
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
			2: { // 有间隙的分区
				PartitionID: 2,
				MinID:       1200, // 间隙：1000+1 到 1199
				MaxID:       2000,
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
		}

		// 保存分区数据
		partitionsData, err := json.Marshal(partitions)
		assert.NoError(t, err)
		err = dataStore.SetPartitions(ctx, worker.getPartitionInfoKey(), string(partitionsData))
		assert.NoError(t, err)

		// 测试是否安全更新
		safe, err := worker.isSafeToUpdateGlobalMaxID(ctx, 2, 2000) // 分区ID 2, 新最大ID 2000
		assert.NoError(t, err)
		assert.True(t, safe, "间隙被已完成分区覆盖时应该安全更新")
	})

	// 测试场景4: 有间隙未覆盖，但都处理完成 - 应该返回安全
	t.Run("GapNotCovered", func(t *testing.T) {
		// 准备分区数据
		partitions := map[int]PartitionInfo{
			0: {
				PartitionID: 0,
				MinID:       100,
				MaxID:       1000, // 全局最大ID
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
			1: { // 有间隙的分区
				PartitionID: 1,
				MinID:       1200, // 间隙：1000+1 到 1199
				MaxID:       2000,
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
		}

		// 保存分区数据
		partitionsData, err := json.Marshal(partitions)
		assert.NoError(t, err)
		err = dataStore.SetPartitions(ctx, worker.getPartitionInfoKey(), string(partitionsData))
		assert.NoError(t, err)

		// 测试是否安全更新
		safe, err := worker.isSafeToUpdateGlobalMaxID(ctx, 1, 2000) // 分区ID 1, 新最大ID 2000
		assert.NoError(t, err)
		assert.True(t, safe, "间隙未覆盖时应该安全更新")
	})

	// 测试场景5: 分区与另一个未完成的分区重叠 - 应该返回不安全
	t.Run("OverlappingWithPendingPartition", func(t *testing.T) {
		// 准备分区数据，包括重叠分区
		partitions := map[int]PartitionInfo{
			0: {
				PartitionID: 0,
				MinID:       100,
				MaxID:       1500,      // 与分区1重叠
				Status:      "pending", // 未完成
				UpdatedAt:   time.Now(),
			},
			1: {
				PartitionID: 1,
				MinID:       1000, // 与分区0重叠
				MaxID:       2000,
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
		}

		// 保存分区数据
		partitionsData, err := json.Marshal(partitions)
		assert.NoError(t, err)
		err = dataStore.SetPartitions(ctx, worker.getPartitionInfoKey(), string(partitionsData))
		assert.NoError(t, err)

		// 测试是否安全更新
		safe, err := worker.isSafeToUpdateGlobalMaxID(ctx, 1, 2000) // 分区ID 1, 新最大ID 2000
		assert.NoError(t, err)
		assert.False(t, safe, "与未完成分区重叠时不应该安全更新")
	})

	// 测试场景6: 当分区不存在时 - 应该返回错误
	t.Run("NonExistingPartition", func(t *testing.T) {
		// 准备分区数据
		partitions := map[int]PartitionInfo{
			0: {
				PartitionID: 0,
				MinID:       100,
				MaxID:       1000,
				Status:      "completed",
				UpdatedAt:   time.Now(),
			},
		}

		// 保存分区数据
		partitionsData, err := json.Marshal(partitions)
		assert.NoError(t, err)
		err = dataStore.SetPartitions(ctx, worker.getPartitionInfoKey(), string(partitionsData))
		assert.NoError(t, err)

		// 测试不存在的分区ID
		_, err = worker.isSafeToUpdateGlobalMaxID(ctx, 999, 2000) // 不存在的分区ID
		assert.Error(t, err, "对不存在的分区应该返回错误")
		assert.Contains(t, err.Error(), "not found", "错误消息应该说明分区未找到")
	})
}
