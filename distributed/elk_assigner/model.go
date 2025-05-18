package elk_assigner

import "time"

// 任务处理相关的常量
const (
	taskRetryDelay     = 1 * time.Second        // 任务获取失败后的重试延迟
	noTaskDelay        = 2 * time.Second        // 无可用任务时的等待时间
	taskCompletedDelay = 500 * time.Millisecond // 任务完成后的延迟，避免立即抢占下一个任务
)

// 分区状态常量
const (
	StatusPending   = "pending"
	StatusClaimed   = "claimed"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

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
