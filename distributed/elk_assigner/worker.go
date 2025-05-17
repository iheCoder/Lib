package elk_assigner

import (
	"Lib/distributed/elk_assigner/data"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"log"
	"os"
	"sync"
	"time"
)

// Constants for distributed coordination
const (
	// Default timing settings
	DefaultLeaderLockExpiry       = 30 * time.Second
	DefaultPartitionLockExpiry    = 3 * time.Minute
	DefaultHeartbeatInterval      = 10 * time.Second
	DefaultLeaderElectionInterval = 5 * time.Second
	DefaultPartitionCount         = 8
	DefaultMaxRetries             = 3
	DefaultConsolidationInterval  = 30 * time.Second

	// Key formats for Redis
	LeaderLockKeyFmt    = "%s:leader_lock"
	PartitionLockFmtFmt = "%s:partition:%d"
	PartitionInfoKeyFmt = "%s:partitions"
	HeartbeatFmtFmt     = "%s:heartbeat:%s"
	StatusKeyFmt        = "%s:status"
	WorkersKeyFmt       = "%s:workers"
)

// TaskProcessor is an interface that task processors must implement
type TaskProcessor interface {
	// Process processes a range of tasks identified by minID and maxID
	// Returns the number of processed items, the final max ID processed, and any error
	Process(ctx context.Context, minID, maxID int64, opts map[string]interface{}) (int64, int64, error)

	// GetMaxProcessedID returns the current max ID that has been successfully processed
	GetMaxProcessedID(ctx context.Context) (int64, error)

	// GetNextMaxID estimates the next batch's max ID based on currentMax
	// The rangeSize parameter suggests how far to look ahead
	GetNextMaxID(ctx context.Context, currentMax int64, rangeSize int64) (int64, error)
}

// Logger interface for logging
type Logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// defaultLogger provides a simple logger implementation
type defaultLogger struct{}

func (l *defaultLogger) Infof(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func (l *defaultLogger) Warnf(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

func (l *defaultLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func (l *defaultLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

// PartitionInfo stores partition information
type PartitionInfo struct {
	PartitionID   int                    `json:"partition_id"`
	MinID         int64                  `json:"min_id"`
	MaxID         int64                  `json:"max_id"`
	WorkerID      string                 `json:"worker_id"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	Status        string                 `json:"status"` // pending, running, completed, failed
	UpdatedAt     time.Time              `json:"updated_at"`
	Options       map[string]interface{} `json:"options,omitempty"` // Optional parameters for the partition
}

// SyncStatus stores global synchronization status
type SyncStatus struct {
	LastCompletedSync time.Time              `json:"last_completed_sync"`
	CurrentLeader     string                 `json:"current_leader"`
	PartitionCount    int                    `json:"partition_count"`
	ActiveWorkers     int                    `json:"active_workers"`
	GlobalMaxID       int64                  `json:"global_max_id"`
	PartitionStatus   map[int]PartitionInfo  `json:"partition_status"`
	AdditionalInfo    map[string]interface{} `json:"additional_info,omitempty"`
}

// Worker represents a distributed worker node
type Worker struct {
	// Configuration
	ID            string
	Namespace     string // Namespace for Redis keys to avoid conflicts
	WorkerOptions map[string]interface{}

	// Components
	// DataStore is the interface for data storage (e.g., Redis)
	DataStore data.DataStore
	// TaskProcessor is the interface for processing tasks
	TaskProcessor TaskProcessor
	// Logger is the interface for logging
	Logger Logger

	// Timing configurations
	LeaderLockExpiry       time.Duration
	PartitionLockExpiry    time.Duration
	HeartbeatInterval      time.Duration
	LeaderElectionInterval time.Duration
	ConsolidationInterval  time.Duration
	MaxRetries             int

	// Internal state
	isLeader        bool
	heartbeatCtx    context.Context
	cancelHeartbeat context.CancelFunc
	leaderCtx       context.Context
	cancelLeader    context.CancelFunc
	workCtx         context.Context
	cancelWork      context.CancelFunc

	// Mutex for thread safety
	mu sync.RWMutex
}

// NewWorker creates a new worker instance
func NewWorker(namespace string, dataStore data.DataStore, processor TaskProcessor) *Worker {
	workerID := generateWorkerID()

	// Create default logger if none provided
	logger := &defaultLogger{}

	return &Worker{
		ID:                     workerID,
		Namespace:              namespace,
		DataStore:              dataStore,
		TaskProcessor:          processor,
		Logger:                 logger,
		WorkerOptions:          make(map[string]interface{}),
		LeaderLockExpiry:       DefaultLeaderLockExpiry,
		PartitionLockExpiry:    DefaultPartitionLockExpiry,
		HeartbeatInterval:      DefaultHeartbeatInterval,
		LeaderElectionInterval: DefaultLeaderElectionInterval,
		ConsolidationInterval:  DefaultConsolidationInterval,
		MaxRetries:             DefaultMaxRetries,
	}
}

// SetLogger sets a custom logger
func (w *Worker) SetLogger(logger Logger) {
	w.Logger = logger
}

// SetOption sets a worker option
func (w *Worker) SetOption(key string, value interface{}) {
	w.WorkerOptions[key] = value
}

// SetTiming configures the worker's timing settings
func (w *Worker) SetTiming(leaderExpiry, partitionExpiry, heartbeat, leaderElection, consolidation time.Duration) {
	if leaderExpiry > 0 {
		w.LeaderLockExpiry = leaderExpiry
	}
	if partitionExpiry > 0 {
		w.PartitionLockExpiry = partitionExpiry
	}
	if heartbeat > 0 {
		w.HeartbeatInterval = heartbeat
	}
	if leaderElection > 0 {
		w.LeaderElectionInterval = leaderElection
	}
	if consolidation > 0 {
		w.ConsolidationInterval = consolidation
	}
}

// Start begins the worker's operation
func (w *Worker) Start(ctx context.Context) error {
	// Create contexts for the worker's components
	w.heartbeatCtx, w.cancelHeartbeat = context.WithCancel(context.Background())
	w.workCtx, w.cancelWork = context.WithCancel(context.Background())

	// Start sending heartbeats
	go w.sendHeartbeat()

	// Try to become the leader
	isLeader, err := w.tryBecomeLeader(ctx)
	if err != nil {
		w.Logger.Errorf("Error during leader election: %v", err)
		// Continue even if leader election fails
	}

	w.mu.Lock()
	w.isLeader = isLeader
	w.mu.Unlock()

	if isLeader {
		w.Logger.Infof("Worker %s became leader", w.ID)
		w.leaderCtx, w.cancelLeader = context.WithCancel(context.Background())
		go w.runLeaderTasks()
	}

	// Start processing partitions
	return w.processWork(ctx)
}

// Stop gracefully stops the worker
func (w *Worker) Stop() {
	w.Logger.Infof("Stopping worker %s", w.ID)

	// Cancel all ongoing tasks
	if w.cancelHeartbeat != nil {
		w.cancelHeartbeat()
	}

	if w.cancelLeader != nil {
		w.cancelLeader()
	}

	if w.cancelWork != nil {
		w.cancelWork()
	}

	// Release any held locks
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.mu.RLock()
	isLeader := w.isLeader
	w.mu.RUnlock()

	if isLeader {
		w.DataStore.ReleaseLock(ctx, w.getLeaderLockKey(), w.ID)
	}

	// Final cleanup
	heartbeatKey := w.getHeartbeatKey()
	w.DataStore.DeleteKey(ctx, heartbeatKey)

	// Unregister worker
	w.DataStore.UnregisterWorker(ctx, w.getWorkersKey(), w.ID, heartbeatKey)

	w.Logger.Infof("Worker %s stopped", w.ID)
}

// IsLeader returns whether this worker is currently the leader
func (w *Worker) IsLeader() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isLeader
}

// generateWorkerID creates a unique worker ID
func generateWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s-%d-%s", hostname, os.Getpid(), uuid.New().String()[:8])
}

// sendHeartbeat periodically sends heartbeat signals
func (w *Worker) sendHeartbeat() {
	heartbeatKey := w.getHeartbeatKey()
	ticker := time.NewTicker(w.HeartbeatInterval)
	defer ticker.Stop()

	// Register worker on start
	ctx := context.Background()
	err := w.DataStore.RegisterWorker(ctx, w.getWorkersKey(), w.ID, heartbeatKey, time.Now().Format(time.RFC3339))
	if err != nil {
		w.Logger.Errorf("Failed to register worker: %v", err)
	}

	for {
		select {
		case <-w.heartbeatCtx.Done():
			w.Logger.Debugf("Heartbeat stopped for worker %s", w.ID)
			return
		case <-ticker.C:
			// Update heartbeat timestamp
			err := w.DataStore.SetHeartbeat(ctx, heartbeatKey, time.Now().Format(time.RFC3339))
			if err != nil {
				w.Logger.Errorf("Failed to send heartbeat: %v", err)
			}
		}
	}
}

// tryBecomeLeader attempts to acquire the leader lock
func (w *Worker) tryBecomeLeader(ctx context.Context) (bool, error) {
	leaderLockKey := w.getLeaderLockKey()
	success, err := w.DataStore.AcquireLock(ctx, leaderLockKey, w.ID, w.LeaderLockExpiry)
	if err != nil {
		return false, errors.Wrap(err, "error acquiring leader lock")
	}

	if success {
		// 在启动renewLeaderLock协程前初始化leaderCtx
		w.leaderCtx, w.cancelLeader = context.WithCancel(context.Background())
		// Start leader lock renewal
		go w.renewLeaderLock()
	}

	return success, nil
}

// renewLeaderLock periodically renews the leader lock
func (w *Worker) renewLeaderLock() {
	leaderLockKey := w.getLeaderLockKey()
	ticker := time.NewTicker(w.LeaderLockExpiry / 3)
	defer ticker.Stop()

	for {
		select {
		case <-w.leaderCtx.Done():
			w.Logger.Debugf("Leader lock renewal stopped for worker %s", w.ID)
			return
		case <-ticker.C:
			ctx := context.Background()
			renewed, err := w.DataStore.RenewLock(ctx, leaderLockKey, w.ID, w.LeaderLockExpiry)
			if err != nil || !renewed {
				w.Logger.Errorf("Failed to renew leader lock: %v, renewed=%v", err, renewed)

				// No longer leader
				w.mu.Lock()
				w.isLeader = false
				w.mu.Unlock()

				return
			}
		}
	}
}

// runLeaderTasks executes leader-specific tasks
func (w *Worker) runLeaderTasks() {
	// Start partition management
	go w.managePartitions()

	// Start global state consolidation
	go w.consolidateGlobalState()
}

// managePartitions manages work partitions as the leader
func (w *Worker) managePartitions() {
	ticker := time.NewTicker(w.LeaderElectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.leaderCtx.Done():
			w.Logger.Debugf("Partition management stopped for worker %s", w.ID)
			return
		case <-ticker.C:
			// Check if still leader
			ctx := context.Background()
			stillLeader, err := w.DataStore.CheckLock(ctx, w.getLeaderLockKey(), w.ID)
			if err != nil || !stillLeader {
				w.Logger.Infof("No longer the leader, stopping partition management")

				w.mu.Lock()
				w.isLeader = false
				w.mu.Unlock()

				return
			}

			// Update work partitions
			if err := w.updateWorkPartitions(ctx); err != nil {
				w.Logger.Errorf("Error updating work partitions: %v", err)
			}
		}
	}
}

// consolidateGlobalState periodically consolidates global state
func (w *Worker) consolidateGlobalState() {
	ticker := time.NewTicker(w.ConsolidationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.leaderCtx.Done():
			w.Logger.Debugf("Global state consolidation stopped for worker %s", w.ID)
			return
		case <-ticker.C:
			ctx := context.Background()

			// Check if still leader
			stillLeader, err := w.DataStore.CheckLock(ctx, w.getLeaderLockKey(), w.ID)
			if err != nil || !stillLeader {
				w.Logger.Infof("No longer the leader, stopping global state consolidation")
				return
			}

			// Try to update global max ID
			if err := w.consolidateMaxID(ctx); err != nil {
				w.Logger.Errorf("Error consolidating global max ID: %v", err)
			}
		}
	}
}

// updateWorkPartitions updates the work partitions
func (w *Worker) updateWorkPartitions(ctx context.Context) error {
	// 1. Get global max ID
	globalMaxID, err := w.TaskProcessor.GetMaxProcessedID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get global max ID")
	}

	// 2. Get active workers
	activeWorkers, err := w.getActiveWorkers(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get active workers")
	}

	// Determine partition count based on active workers
	partitionCount := len(activeWorkers)
	if partitionCount == 0 {
		partitionCount = 1 // At least one partition
	} else if partitionCount > DefaultPartitionCount {
		partitionCount = DefaultPartitionCount // Limit max partitions
	}

	// 3. Get current partitions
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		// If retrieval fails, create new partitions
		partitions = make(map[int]PartitionInfo)
	}

	// 4. Determine if partitions need updating
	needUpdate := w.needUpdatePartitions(ctx, partitions, partitionCount, globalMaxID)
	if !needUpdate {
		w.Logger.Debugf("No need to update partitions")
		return nil
	}

	// 5. Calculate next max ID to sync
	maxIDToSync, err := w.TaskProcessor.GetNextMaxID(ctx, globalMaxID, 100_000)
	if err != nil {
		w.Logger.Warnf("Failed to get next batch max ID: %v, using globalMaxID + 1000000", err)
		maxIDToSync = globalMaxID + 1000000
	}

	// Ensure maxIDToSync is greater than globalMaxID
	if maxIDToSync <= globalMaxID {
		maxIDToSync = globalMaxID + 1000000
	}

	w.Logger.Infof("Updating partitions: active workers=%d, partition count=%d, ID range=[%d, %d]",
		len(activeWorkers), partitionCount, globalMaxID, maxIDToSync)

	// 6. Calculate new partitions
	newPartitions := w.calculatePartitions(partitionCount, globalMaxID, maxIDToSync, partitions, activeWorkers)

	// 7. Save new partitions
	return w.savePartitions(ctx, newPartitions, globalMaxID, len(activeWorkers))
}

// needUpdatePartitions determines if partitions need to be updated
func (w *Worker) needUpdatePartitions(ctx context.Context, currentPartitions map[int]PartitionInfo, desiredPartitionCount int, globalMaxID int64) bool {
	// 1. If no partitions exist, create new ones
	if len(currentPartitions) == 0 {
		return true
	}

	// 2. If partition count mismatch, update needed
	if len(currentPartitions) != desiredPartitionCount {
		return true
	}

	// 3. Count partitions by status
	pending, running, completed, failed := 0, 0, 0, 0

	for _, partition := range currentPartitions {
		switch partition.Status {
		case "pending":
			pending++
		case "running":
			running++
			// Check if running partition is processing already-synced IDs
			if partition.MinID < globalMaxID {
				w.Logger.Infof(
					"Found running partition with outdated range: partition_id=%d, range=[%d, %d], current globalMaxID=%d",
					partition.PartitionID, partition.MinID, partition.MaxID, globalMaxID)
				return true
			}
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}

	// 4. If all partitions are complete or failed, create new ones
	if completed+failed == len(currentPartitions) {
		w.Logger.Infof("All partitions completed or failed, creating new partitions")
		return true
	}

	// 5. If there are pending partitions but none running, refresh
	if pending > 0 && running == 0 {
		w.Logger.Infof("Found pending partitions but no running partitions, refreshing")
		return true
	}

	// 6. Check for stale running partitions
	now := time.Now()
	for _, partition := range currentPartitions {
		if partition.Status == "running" {
			if now.Sub(partition.UpdatedAt) > w.PartitionLockExpiry*2 {
				w.Logger.Infof(
					"Found stale running partition: partition_id=%d, last_updated=%v, age=%v",
					partition.PartitionID, partition.UpdatedAt, now.Sub(partition.UpdatedAt))
				return true
			}
		}
	}

	return false
}

// getActiveWorkers retrieves active worker list
func (w *Worker) getActiveWorkers(ctx context.Context) ([]string, error) {
	// Use Redis keys to get all heartbeat keys
	pattern := fmt.Sprintf(w.getHeartbeatPattern(), "*")
	keys, err := w.DataStore.GetKeys(ctx, pattern)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get heartbeat keys")
	}

	// Check each heartbeat for validity
	var activeWorkers []string
	now := time.Now()
	validHeartbeatDuration := w.HeartbeatInterval * 3

	for _, key := range keys {
		// Extract worker ID from key
		prefix := fmt.Sprintf(HeartbeatFmtFmt, w.Namespace, "")
		workerID := key[len(prefix):]

		// Get last heartbeat time
		lastHeartbeatStr, err := w.DataStore.GetHeartbeat(ctx, key)
		if err != nil {
			continue // Skip error heartbeats
		}

		lastHeartbeat, err := time.Parse(time.RFC3339, lastHeartbeatStr)
		if err != nil {
			continue // Skip invalid time formats
		}

		// Check if heartbeat is valid
		if now.Sub(lastHeartbeat) <= validHeartbeatDuration {
			activeWorkers = append(activeWorkers, workerID)
		} else {
			// Remove expired heartbeat
			w.DataStore.DeleteKey(ctx, key)
		}
	}

	return activeWorkers, nil
}

// getCurrentPartitions gets the current partition information
func (w *Worker) getCurrentPartitions(ctx context.Context) (map[int]PartitionInfo, error) {
	partitionsData, err := w.DataStore.GetPartitions(ctx, w.getPartitionInfoKey())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get partitions")
	}

	var partitions map[int]PartitionInfo
	if err := json.Unmarshal([]byte(partitionsData), &partitions); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal partitions data")
	}

	return partitions, nil
}

// calculatePartitions creates new partition assignments
func (w *Worker) calculatePartitions(partitionCount int, globalMaxID, maxIDToSync int64,
	currentPartitions map[int]PartitionInfo, activeWorkers []string) map[int]PartitionInfo {

	newPartitions := make(map[int]PartitionInfo)

	// Calculate range for each partition
	idRange := maxIDToSync - globalMaxID
	partitionSize := idRange / int64(partitionCount)
	if partitionSize < 1000 {
		partitionSize = 1000 // Ensure minimum size
	}

	// Create partitions
	for i := 0; i < partitionCount; i++ {
		minID := globalMaxID + int64(i)*partitionSize
		maxID := globalMaxID + int64(i+1)*partitionSize
		if i == partitionCount-1 {
			maxID = maxIDToSync // Last partition goes to max
		}

		// Check if current partition exists and is valid
		existingPartition, exists := currentPartitions[i]
		if exists && existingPartition.Status == "running" {
			// Verify worker is still active
			workerActive := false
			for _, worker := range activeWorkers {
				if worker == existingPartition.WorkerID {
					workerActive = true
					break
				}
			}

			if workerActive {
				// Keep existing partition
				existingPartition.UpdatedAt = time.Now()
				newPartitions[i] = existingPartition
				continue
			}
		}

		// Create new partition
		newPartitions[i] = PartitionInfo{
			PartitionID: i,
			MinID:       minID,
			MaxID:       maxID,
			WorkerID:    "", // Empty until claimed
			Status:      "pending",
			UpdatedAt:   time.Now(),
			Options:     make(map[string]interface{}),
		}

		// Copy any global options
		for k, v := range w.WorkerOptions {
			newPartitions[i].Options[k] = v
		}
	}

	return newPartitions
}

// savePartitions saves partition information
func (w *Worker) savePartitions(ctx context.Context, partitions map[int]PartitionInfo, globalMaxID int64, activeWorkerCount int) error {
	// 1. Save partition info
	partitionsData, err := json.Marshal(partitions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal partitions")
	}

	if err := w.DataStore.SetPartitions(ctx, w.getPartitionInfoKey(), string(partitionsData)); err != nil {
		return errors.Wrap(err, "failed to set partitions data")
	}

	// 2. Update sync status
	status := SyncStatus{
		LastCompletedSync: time.Now(),
		PartitionCount:    len(partitions),
		ActiveWorkers:     activeWorkerCount,
		GlobalMaxID:       globalMaxID,
		PartitionStatus:   partitions,
		AdditionalInfo:    make(map[string]interface{}),
	}

	// Add worker options to additional info
	for k, v := range w.WorkerOptions {
		status.AdditionalInfo[k] = v
	}

	// Get current leader
	leaderID, err := w.DataStore.GetLockOwner(ctx, w.getLeaderLockKey())
	if err == nil {
		status.CurrentLeader = leaderID
	}

	statusData, err := json.Marshal(status)
	if err != nil {
		return errors.Wrap(err, "failed to marshal status")
	}

	return w.DataStore.SetSyncStatus(ctx, w.getStatusKey(), string(statusData))
}

// processWork processes assigned work partitions
func (w *Worker) processWork(ctx context.Context) error {
	// 持续尝试处理分区直到上下文取消
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.workCtx.Done():
			return w.workCtx.Err()
		default:
			// 尝试获取并处理一个分区
			partition, err := w.claimPartition(ctx)
			if err == nil && partition != nil {
				// 成功获取到分区，处理它
				w.Logger.Infof("成功获取分区 %d, 范围 [%d, %d]", partition.PartitionID, partition.MinID, partition.MaxID)
				if processErr := w.processPartition(ctx, partition); processErr != nil {
					w.Logger.Errorf("处理分区 %d 时出错: %v", partition.PartitionID, processErr)

					// 出错时也要尝试更新分区状态为失败
					updateErr := w.updatePartitionStatus(ctx, partition.PartitionID, "failed")
					if updateErr != nil {
						w.Logger.Errorf("无法将分区 %d 状态更新为失败: %v", partition.PartitionID, updateErr)
					}

					// 释放分区锁，让其他 worker 可以处理
					lockKey := w.getPartitionLockKey(partition.PartitionID)
					w.releaseLock(ctx, lockKey)
				}
				// 短暂休息后继续主循环
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 如果没有可用分区，等待后重试
			if err != nil && errors.Is(err, ErrNoAvailablePartition) {
				w.Logger.Debugf("没有可用分区，等待 %s", w.HeartbeatInterval)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-w.workCtx.Done():
					return w.workCtx.Err()
				case <-time.After(w.HeartbeatInterval):
					// 继续尝试
				}
			} else if err != nil {
				w.Logger.Warnf("获取分区时出错: %v", err)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-w.workCtx.Done():
					return w.workCtx.Err()
				case <-time.After(w.HeartbeatInterval / 2):
					// 继续尝试，但等待时间较短
				}
			}
		}
	}
}

// claimPartition attempts to claim an available partition
func (w *Worker) claimPartition(ctx context.Context) (*PartitionInfo, error) {
	// Get current partitions
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current partitions")
	}

	// Look for pending partitions
	for i, partition := range partitions {
		if partition.Status == "pending" && partition.WorkerID == "" {
			// Try to lock this partition
			lockKey := w.getPartitionLockKey(i)
			success, err := w.DataStore.AcquireLock(ctx, lockKey, w.ID, w.PartitionLockExpiry)
			if err != nil {
				w.Logger.Warnf("Error acquiring lock for partition %d: %v", i, err)
				continue
			}

			if success {
				// Update partition info
				partition.WorkerID = w.ID
				partition.LastHeartbeat = time.Now()
				partition.UpdatedAt = time.Now()

				// Save updated partition
				partitions[i] = partition
				partitionsData, err := json.Marshal(partitions)
				if err != nil {
					w.releaseLock(ctx, lockKey)
					return nil, errors.Wrap(err, "failed to marshal updated partitions")
				}

				if err := w.DataStore.SetPartitions(ctx, w.getPartitionInfoKey(), string(partitionsData)); err != nil {
					w.releaseLock(ctx, lockKey)
					return nil, errors.Wrap(err, "failed to save updated partitions")
				}

				return &partition, nil
			}
		}
	}

	return nil, ErrNoAvailablePartition
}

// processPartition processes a claimed partition
func (w *Worker) processPartition(ctx context.Context, partition *PartitionInfo) error {
	w.Logger.Infof("开始处理分区 %d，ID范围 [%d, %d]",
		partition.PartitionID, partition.MinID, partition.MaxID)

	// 在options中添加partition_id，帮助跟踪处理
	if partition.Options == nil {
		partition.Options = make(map[string]interface{})
	}
	partition.Options["partition_id"] = partition.PartitionID
	partition.Options["worker_id"] = w.ID

	// 更新分区状态为运行中
	if err := w.updatePartitionStatus(ctx, partition.PartitionID, "running"); err != nil {
		w.Logger.Errorf("更新分区 %d 状态为 running 失败: %v", partition.PartitionID, err)
		// 尽管有错误，继续处理
	}

	// 使用任务处理器处理分区
	w.Logger.Infof("调用处理器处理分区 %d", partition.PartitionID)
	processCount, finalMaxID, err := w.TaskProcessor.Process(
		ctx,
		partition.MinID,
		partition.MaxID,
		partition.Options,
	)
	w.Logger.Infof("处理器返回：分区 %d，处理项目数 %d，最终maxID %d，错误: %v",
		partition.PartitionID, processCount, finalMaxID, err)

	// 根据结果更新分区状态
	status := "completed"
	if err != nil {
		status = "failed"
		w.Logger.Errorf("分区 %d 处理失败: %v", partition.PartitionID, err)
	}

	// 更新分区状态
	w.Logger.Infof("正在将分区 %d 状态更新为 %s", partition.PartitionID, status)
	if updateErr := w.updatePartitionStatus(ctx, partition.PartitionID, status); updateErr != nil {
		w.Logger.Errorf("更新分区 %d 状态为 %s 失败: %v", partition.PartitionID, status, updateErr)
		// 这里我们仍然继续，以便解锁分区
	} else {
		w.Logger.Infof("成功更新分区 %d 状态为 %s", partition.PartitionID, status)
	}

	// 释放分区锁
	lockKey := w.getPartitionLockKey(partition.PartitionID)
	w.Logger.Infof("正在释放分区 %d 锁", partition.PartitionID)
	w.releaseLock(ctx, lockKey)

	// 状态为completed时尝试更新全局状态
	if err == nil && status == "completed" {
		// 使用合理的最终ID值
		effectiveMaxID := finalMaxID

		// 如果返回的finalMaxID无效或小于分区的maxID，使用分区的maxID
		// 修复：确保空分区或无数据分区也能更新全局状态
		if effectiveMaxID < partition.MaxID {
			effectiveMaxID = partition.MaxID
			w.Logger.Infof("使用分区最大值 %d 作为有效最大ID值 (处理计数=%d, 返回的最大ID=%d)",
				effectiveMaxID, processCount, finalMaxID)
		}

		// 检查是否可以安全更新全局最大ID
		safeToUpdate, checkErr := w.isSafeToUpdateGlobalMaxID(ctx, partition.PartitionID, effectiveMaxID)
		if checkErr != nil {
			w.Logger.Warnf("检查更新全局maxID安全性时出错: %v", checkErr)
		} else if safeToUpdate {
			// 使用专用方法更新
			w.Logger.Infof("正在尝试更新全局maxID为 %d (分区 %d)", effectiveMaxID, partition.PartitionID)
			if updateErr := w.updateGlobalMaxID(ctx, effectiveMaxID); updateErr != nil {
				w.Logger.Errorf("更新全局maxID失败: %v", updateErr)
			} else {
				w.Logger.Infof("成功更新全局maxID为 %d (分区 %d)",
					effectiveMaxID, partition.PartitionID)
			}
		} else {
			w.Logger.Infof("跳过更新全局maxID为 %d (分区 %d): 较低分区仍在进行中",
				effectiveMaxID, partition.PartitionID)

			// 即使现在不是安全的，也尝试合并全局状态，捕获任何可能已完成的序列
			consolidateErr := w.consolidateMaxID(ctx)
			if consolidateErr != nil {
				w.Logger.Warnf("合并全局maxID失败: %v", consolidateErr)
			}
		}
	}

	return err
}

// updatePartitionStatus updates a partition's status
func (w *Worker) updatePartitionStatus(ctx context.Context, partitionID int, status string) error {
	// Get current partitions
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current partitions")
	}

	// Update status
	partition, exists := partitions[partitionID]
	if !exists {
		return fmt.Errorf("partition %d not found", partitionID)
	}

	partition.Status = status
	partition.UpdatedAt = time.Now()
	partitions[partitionID] = partition

	// Save updated partitions
	partitionsData, err := json.Marshal(partitions)
	if err != nil {
		return errors.Wrap(err, "failed to marshal updated partitions")
	}

	return w.DataStore.SetPartitions(ctx, w.getPartitionInfoKey(), string(partitionsData))
}

// consolidateMaxID consolidates the global max ID
func (w *Worker) consolidateMaxID(ctx context.Context) error {
	// 获取当前全局最大ID
	currentMaxID, err := w.TaskProcessor.GetMaxProcessedID(ctx)
	if err != nil {
		return errors.Wrap(err, "无法获取当前全局maxID")
	}

	// 获取所有分区信息
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		return errors.Wrap(err, "无法获取分区")
	}

	// 没有分区，无需处理
	if len(partitions) == 0 {
		w.Logger.Debugf("没有分区需要合并")
		return nil
	}

	w.Logger.Infof("开始合并全局maxID，当前值=%d, 分区数=%d", currentMaxID, len(partitions))

	// 检查所有分区状态
	allCompleted := true
	failedCount := 0
	pendingCount := 0
	runningCount := 0
	completedCount := 0
	maxPartitionID := currentMaxID

	// 找出最大的完成分区和最后一个分区
	var maxCompletedPartition, lastPartition *PartitionInfo

	for _, partition := range partitions {
		switch partition.Status {
		case "completed":
			completedCount++
			if maxCompletedPartition == nil || partition.MaxID > maxCompletedPartition.MaxID {
				copied := partition // 创建副本以避免指针问题
				maxCompletedPartition = &copied
			}
			if partition.MaxID > maxPartitionID {
				maxPartitionID = partition.MaxID
			}
		case "failed":
			failedCount++
		case "pending":
			pendingCount++
			allCompleted = false
		case "running":
			runningCount++
			allCompleted = false
		default:
			allCompleted = false
		}

		// 记录最后一个分区(最大MaxID的分区)
		if lastPartition == nil || partition.MaxID > lastPartition.MaxID {
			copied := partition // 创建副本以避免指针问题
			lastPartition = &copied
		}
	}

	w.Logger.Infof("分区状态统计: 完成=%d, 失败=%d, 待处理=%d, 运行中=%d",
		completedCount, failedCount, pendingCount, runningCount)

	// 特殊情况1: 所有分区都已完成或失败，直接更新到最大值
	if allCompleted && maxPartitionID > currentMaxID {
		w.Logger.Infof("所有分区已处理，尝试更新全局maxID从 %d 到 %d",
			currentMaxID, maxPartitionID)

		if err := w.updateGlobalMaxID(ctx, maxPartitionID); err != nil {
			return errors.Wrap(err, "无法更新所有已完成分区的全局maxID")
		}

		w.Logger.Infof("成功更新全局maxID从 %d 到 %d", currentMaxID, maxPartitionID)
		return nil
	}

	// 特殊情况2: 最后一个分区已完成但全局maxID未更新
	if lastPartition != nil && lastPartition.Status == "completed" &&
		maxCompletedPartition != nil && maxCompletedPartition.MaxID > currentMaxID {
		w.Logger.Infof("最后分区已完成，尝试更新全局maxID从 %d 到 %d",
			currentMaxID, maxCompletedPartition.MaxID)

		if err := w.updateGlobalMaxID(ctx, maxCompletedPartition.MaxID); err != nil {
			return errors.Wrap(err, "无法更新最后完成分区的全局maxID")
		}

		w.Logger.Infof("成功更新全局maxID从 %d 到 %d（最后分区已完成）",
			currentMaxID, maxCompletedPartition.MaxID)
		return nil
	}

	// 寻找从当前maxID开始的连续已完成分区
	nextExpectedID := currentMaxID + 1
	newMaxID := currentMaxID
	continueSearch := true

	for continueSearch {
		partitionFound := false

		for _, partition := range partitions {
			// 考虑已完成和失败的分区
			if partition.Status != "completed" && partition.Status != "failed" {
				continue
			}

			// 检查分区是否覆盖nextExpectedID
			if partition.MinID <= nextExpectedID && partition.MaxID >= nextExpectedID {
				partitionFound = true

				// 更新nextExpectedID和潜在的newMaxID
				if partition.Status == "completed" && partition.MaxID > newMaxID {
					newMaxID = partition.MaxID
					nextExpectedID = newMaxID + 1
					w.Logger.Debugf("发现连续分区 %d，更新潜在的最大ID为 %d",
						partition.PartitionID, newMaxID)
				} else if partition.Status == "failed" {
					// 对于失败的分区，我们向前移动到分区结束
					nextExpectedID = partition.MaxID + 1
					w.Logger.Debugf("跳过失败的分区 %d，继续从 %d",
						partition.PartitionID, nextExpectedID)
				}

				break
			}
		}

		// 如果本轮没找到合适分区，停止搜索
		if !partitionFound {
			continueSearch = false
		}
	}

	// 如果newMaxID大于当前值，更新它
	if newMaxID > currentMaxID {
		w.Logger.Infof("尝试合并全局maxID从 %d 到 %d", currentMaxID, newMaxID)

		if err := w.updateGlobalMaxID(ctx, newMaxID); err != nil {
			return errors.Wrap(err, "无法更新全局maxID")
		}

		w.Logger.Infof("成功合并全局maxID从 %d 到 %d", currentMaxID, newMaxID)
	} else {
		w.Logger.Debugf("无需更新全局maxID，当前值 %d", currentMaxID)
	}

	return nil
}

// updateGlobalMaxID updates the global maximum processed ID
func (w *Worker) updateGlobalMaxID(ctx context.Context, newMaxID int64) error {
	// This should either call a dedicated method on TaskProcessor
	// or pass a special flag to Process, but not overload Process with unrelated parameters

	// Option 1: Call a dedicated method if available on your TaskProcessor implementation
	if updater, ok := w.TaskProcessor.(interface {
		UpdateMaxProcessedID(context.Context, int64) error
	}); ok {
		return updater.UpdateMaxProcessedID(ctx, newMaxID)
	}

	// Option 2: Fall back to using Process with special parameters
	// This is a temporary solution until you refactor the TaskProcessor interface
	_, _, err := w.TaskProcessor.Process(ctx, 0, 0, map[string]interface{}{
		"action":     "update_max_id",
		"new_max_id": newMaxID,
	})
	return err
}

// isSafeToUpdateGlobalMaxID checks if it's safe to update the global max ID
func (w *Worker) isSafeToUpdateGlobalMaxID(ctx context.Context, partitionID int, newMaxID int64) (bool, error) {
	// 获取当前分区状态
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get partitions")
	}

	// 获取当前分区信息
	currentPartition, exists := partitions[partitionID]
	if !exists {
		return false, fmt.Errorf("partition %d not found", partitionID)
	}

	currentMinID := currentPartition.MinID

	// 判断是否为最后一个分区
	isLastPartition := true
	for id, partition := range partitions {
		if id != partitionID && partition.MaxID > currentPartition.MaxID {
			isLastPartition = false
			break
		}
	}

	// 特殊情况1: 最后一个分区总是允许更新
	if isLastPartition && currentPartition.Status == "completed" {
		w.Logger.Infof("允许最后分区 %d (范围 %d-%d) 更新全局maxID",
			partitionID, currentPartition.MinID, currentPartition.MaxID)
		return true, nil
	}

	// 特殊情况2: 空分区或单个ID分区也总是允许更新
	if currentPartition.MinID >= currentPartition.MaxID && currentPartition.Status == "completed" {
		w.Logger.Infof("允许空/单ID分区 %d (ID=%d) 更新全局maxID",
			partitionID, currentPartition.MinID)
		return true, nil
	}

	// 检查是否有更低范围的分区尚未完成
	for id, partition := range partitions {
		// 跳过当前分区
		if id == partitionID {
			continue
		}

		// 检查分区是否有更低的范围
		if partition.MaxID < currentMinID {
			// 如果这个较低的分区尚未完成，则不安全更新
			if partition.Status != "completed" && partition.Status != "failed" {
				w.Logger.Infof("无法更新全局maxID: 分区 %d (范围 %d-%d) 状态为 %s 阻止更新",
					id, partition.MinID, partition.MaxID, partition.Status)
				return false, nil
			}
		}
	}

	// 获取当前全局最大ID以检查间隙
	currentGlobalMaxID, err := w.TaskProcessor.GetMaxProcessedID(ctx)
	if err != nil {
		return false, errors.Wrap(err, "无法获取当前全局maxID")
	}

	// 特殊情况3: 当前分区直接接续全局最大ID
	if currentMinID == currentGlobalMaxID+1 {
		w.Logger.Infof("分区 %d 直接跟随全局maxID %d, 允许更新到 %d",
			partitionID, currentGlobalMaxID, newMaxID)
		return true, nil
	}

	// 如果当前全局最大ID和分区最小ID之间存在间隙，检查中间分区
	if currentMinID > currentGlobalMaxID+1 {
		// 检查是否有覆盖间隙的已完成分区
		gapCovered := false

		for _, partition := range partitions {
			// 跳过当前分区
			if partition.PartitionID == partitionID {
				continue
			}

			// 检查分区是否覆盖间隙
			if partition.MinID <= currentGlobalMaxID+1 && partition.MaxID >= currentMinID-1 {
				if partition.Status == "completed" || partition.Status == "failed" {
					gapCovered = true
					w.Logger.Infof("全局maxID %d 与分区 %d 最小ID %d 之间的间隙被已完成分区 %d 覆盖",
						currentGlobalMaxID, partitionID, currentMinID, partition.PartitionID)
					break
				} else {
					// 有分区覆盖间隙但未完成，不安全更新
					w.Logger.Infof("无法更新全局maxID: 中间分区 %d (范围 %d-%d) 状态为 %s 阻止更新",
						partition.PartitionID, partition.MinID, partition.MaxID, partition.Status)
					return false, nil
				}
			}
		}

		// 特殊情况4: 即使有间隙，如果这是最后一个分区也允许更新
		if !gapCovered && isLastPartition {
			w.Logger.Infof("最后分区 %d 在全局maxID %d 之后有间隙，但允许更新，因为它是最终分区",
				partitionID, currentGlobalMaxID)
			return true, nil
		}

		if !gapCovered {
			w.Logger.Infof("无法更新全局maxID: 全局maxID %d 与分区 %d 最小ID %d 之间的间隙未覆盖",
				currentGlobalMaxID, partitionID, currentMinID)
			return false, nil
		}
	}

	return true, nil
}

// releaseLock releases a lock
func (w *Worker) releaseLock(ctx context.Context, key string) {
	if err := w.DataStore.ReleaseLock(ctx, key, w.ID); err != nil {
		w.Logger.Warnf("Failed to release lock %s: %v", key, err)
	}
}

// getLeaderLockKey returns the leader lock key
func (w *Worker) getLeaderLockKey() string {
	return fmt.Sprintf(LeaderLockKeyFmt, w.Namespace)
}

// getPartitionLockKey returns the partition lock key
func (w *Worker) getPartitionLockKey(partitionID int) string {
	return fmt.Sprintf(PartitionLockFmtFmt, w.Namespace, partitionID)
}

// getPartitionInfoKey returns the partition info key
func (w *Worker) getPartitionInfoKey() string {
	return fmt.Sprintf(PartitionInfoKeyFmt, w.Namespace)
}

// getHeartbeatKey returns the worker's heartbeat key
func (w *Worker) getHeartbeatKey() string {
	return fmt.Sprintf(HeartbeatFmtFmt, w.Namespace, w.ID)
}

// getHeartbeatPattern returns the pattern for all heartbeat keys
func (w *Worker) getHeartbeatPattern() string {
	return fmt.Sprintf(HeartbeatFmtFmt, w.Namespace, "%s")
}

// getStatusKey returns the status key
func (w *Worker) getStatusKey() string {
	return fmt.Sprintf(StatusKeyFmt, w.Namespace)
}

// getWorkersKey returns the workers key
func (w *Worker) getWorkersKey() string {
	return fmt.Sprintf(WorkersKeyFmt, w.Namespace)
}

// Custom errors
var (
	ErrNoAvailablePartition = errors.New("no available partition to claim")
	ErrMaxRetriesExceeded   = errors.New("maximum retry attempts exceeded")
)
