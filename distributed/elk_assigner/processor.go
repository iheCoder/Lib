package elk_assigner

import "context"

// Processor is an interface that task processors must implement
type Processor interface {
	// Process processes a range of tasks identified by minID and maxID
	// Returns the number of processed items and any error
	Process(ctx context.Context, minID, maxID int64, opts map[string]interface{}) (int64, error)

	// GetLastProcessedID returns the last processed ID that has been successfully processed
	GetLastProcessedID(ctx context.Context) (int64, error)

	// GetNextMaxID estimates the next batch's max ID based on currentMax
	// The rangeSize parameter suggests how far to look ahead
	GetNextMaxID(ctx context.Context, startID int64, rangeSize int64) (int64, error)
}
