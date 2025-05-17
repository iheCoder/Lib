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
	maxIDToSync, err := w.TaskProcessor.GetNextMaxID(ctx, globalMaxID, 100000)
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
	for i := 0; i < w.MaxRetries; i++ {
		// Try to claim a partition
		partition, err := w.claimPartition(ctx)
		if err == nil && partition != nil {
			// Process the partition
			return w.processPartition(ctx, partition)
		}

		// If no partition available, wait before retry
		if err != nil && errors.Is(err, ErrNoAvailablePartition) {
			w.Logger.Infof("No available partition, waiting for %s", w.HeartbeatInterval*3)
			time.Sleep(w.HeartbeatInterval * 3)
		} else if err != nil {
			w.Logger.Warnf("Error claiming partition: %v", err)
			time.Sleep(w.HeartbeatInterval)
		}
	}

	w.Logger.Warnf("Failed to claim a partition after %d attempts", w.MaxRetries)
	return ErrMaxRetriesExceeded
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
	w.Logger.Infof("Processing partition %d with ID range [%d, %d]",
		partition.PartitionID, partition.MinID, partition.MaxID)

	// Update partition status to running
	if err := w.updatePartitionStatus(ctx, partition.PartitionID, "running"); err != nil {
		w.Logger.Errorf("Failed to update partition status: %v", err)
		// Continue processing
	}

	// Process the partition using the task processor
	processCount, finalMaxID, err := w.TaskProcessor.Process(
		ctx,
		partition.MinID,
		partition.MaxID,
		partition.Options,
	)

	// Update partition status based on result
	status := "completed"
	if err != nil {
		status = "failed"
		w.Logger.Errorf("Partition %d processing failed: %v", partition.PartitionID, err)
	}

	// Update partition status
	if updateErr := w.updatePartitionStatus(ctx, partition.PartitionID, status); updateErr != nil {
		w.Logger.Errorf("Failed to update partition status to %s: %v", status, updateErr)
	}

	w.Logger.Infof("Completed processing partition %d: processed %d items, final maxID %d",
		partition.PartitionID, processCount, finalMaxID)

	// Release partition lock
	lockKey := w.getPartitionLockKey(partition.PartitionID)
	w.releaseLock(ctx, lockKey)

	// If successful and items were processed, check if we can update global max ID
	if err == nil && processCount > 0 && finalMaxID > partition.MinID {
		safeToUpdate, checkErr := w.isSafeToUpdateGlobalMaxID(ctx, partition.PartitionID, finalMaxID)
		if checkErr != nil {
			w.Logger.Warnf("Failed to check if safe to update global maxID: %v", checkErr)
		} else if safeToUpdate {
			// Update using the dedicated method instead
			if updateErr := w.updateGlobalMaxID(ctx, finalMaxID); updateErr != nil {
				w.Logger.Errorf("Failed to update global maxID: %v", updateErr)
			} else {
				w.Logger.Infof("Updated global maxID to %d", finalMaxID)
			}
		} else {
			w.Logger.Infof("Skipping global maxID update to %d as lower partitions are still in progress", finalMaxID)
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
	// Get current global max ID
	currentMaxID, err := w.TaskProcessor.GetMaxProcessedID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current global maxID")
	}

	// Get all partition information
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get partitions")
	}

	// No partitions, nothing to do
	if len(partitions) == 0 {
		return nil
	}

	// Find continuous completed partitions starting from current maxID
	nextExpectedID := currentMaxID + 1
	newMaxID := currentMaxID
	continueSearch := true

	for continueSearch {
		partitionFound := false

		for _, partition := range partitions {
			// Only consider completed partitions
			if partition.Status != "completed" {
				continue
			}

			// Check if this partition covers nextExpectedID
			if partition.MinID <= nextExpectedID && partition.MaxID >= nextExpectedID {
				// Found a completed partition covering nextExpectedID
				partitionFound = true

				// Update nextExpectedID and potentially newMaxID
				if partition.MaxID > newMaxID {
					newMaxID = partition.MaxID
					nextExpectedID = newMaxID + 1
				}

				break
			}
		}

		// If no suitable partition found in this round, stop searching
		if !partitionFound {
			continueSearch = false
		}
	}

	// If newMaxID is greater than current, update it
	if newMaxID > currentMaxID {
		// Using TaskProcessor.Process with action=update_max_id is incorrect
		// We should directly call a dedicated method for updating max ID
		if err := w.updateGlobalMaxID(ctx, newMaxID); err != nil {
			return errors.Wrap(err, "failed to update global maxID")
		}

		w.Logger.Infof("Consolidated global maxID from %d to %d", currentMaxID, newMaxID)
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
	// Get current partitions
	partitions, err := w.getCurrentPartitions(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get partitions")
	}

	// Get current partition's minID
	currentPartition, exists := partitions[partitionID]
	if !exists {
		return false, fmt.Errorf("partition %d not found", partitionID)
	}

	currentMinID := currentPartition.MinID

	// Check for any lower partitions that aren't completed
	for id, partition := range partitions {
		// Skip current partition
		if id == partitionID {
			continue
		}

		// Check if this partition has a lower range
		if partition.MaxID <= currentMinID {
			// If this lower partition isn't completed, not safe to update
			if partition.Status != "completed" {
				w.Logger.Infof("Cannot update global maxID: partition %d (range %d-%d) with status %s blocks update",
					id, partition.MinID, partition.MaxID, partition.Status)
				return false, nil
			}
		}
	}

	// Get current global max ID to check for gaps
	currentGlobalMaxID, err := w.TaskProcessor.GetMaxProcessedID(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get current global maxID")
	}

	// If there's a gap between current global max and partition min, check intermediate partitions
	if currentMinID > currentGlobalMaxID+1 {
		for id, partition := range partitions {
			// Skip current partition
			if id == partitionID {
				continue
			}

			// Check if partition covers the gap
			if partition.MinID <= currentGlobalMaxID && partition.MaxID >= currentMinID {
				// If this partition isn't completed, not safe to update
				if partition.Status != "completed" {
					w.Logger.Infof("Cannot update global maxID: intermediate partition %d (range %d-%d) with status %s blocks update",
						id, partition.MinID, partition.MaxID, partition.Status)
					return false, nil
				}
			}
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
