package redisstate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// StepEvent is the normalized message broadcast to the frontend's trace panel
// over the /ws/executions/:run_id websocket, mirroring TraceEvent in
// py-planner's planner/models.py so both the SSE stream and the websocket
// stream render identically in TracePanel.tsx.
type StepEvent struct {
	Type          string          `json:"type"` // step_created | step_executing | step_result | step_error | run_status_changed
	RunID         string          `json:"run_id"`
	StepID        string          `json:"step_id,omitempty"`
	ToolName      string          `json:"tool_name,omitempty"`
	Status        string          `json:"status,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
}

func channelName(runID string) string {
	return fmt.Sprintf("integradock:execution:%s", runID)
}

// Publish broadcasts a StepEvent on the run's Redis pub/sub channel.
// Any error is non-fatal to the caller's main flow (execution should not
// fail just because no one is currently listening on the websocket).
func Publish(ctx context.Context, rdb *redis.Client, runID string, event StepEvent) error {
	event.RunID = runID
	event.Timestamp = time.Now().UTC()

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("redisstate: failed to marshal event: %w", err)
	}

	if err := rdb.Publish(ctx, channelName(runID), payload).Err(); err != nil {
		return fmt.Errorf("redisstate: failed to publish event: %w", err)
	}
	return nil
}

// Subscribe opens a Redis pub/sub subscription for a run's channel.
// Caller is responsible for closing the returned *redis.PubSub.
func Subscribe(ctx context.Context, rdb *redis.Client, runID string) *redis.PubSub {
	return rdb.Subscribe(ctx, channelName(runID))
}

// SetRunState caches the last-known run status in Redis with a short TTL,
// so a websocket client connecting mid-run can be told the current state
// immediately instead of waiting for the next event.
func SetRunState(ctx context.Context, rdb *redis.Client, runID, status string) error {
	key := fmt.Sprintf("integradock:run_state:%s", runID)
	return rdb.Set(ctx, key, status, 1*time.Hour).Err()
}

// GetRunState reads the cached run status, if any.
func GetRunState(ctx context.Context, rdb *redis.Client, runID string) (string, error) {
	key := fmt.Sprintf("integradock:run_state:%s", runID)
	val, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}