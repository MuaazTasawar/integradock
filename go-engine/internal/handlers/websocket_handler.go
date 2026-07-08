package handlers

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/redis/go-redis/v9"

	"github.com/MuaazTasawar/integradock/go-engine/internal/redisstate"
)

// WebsocketHandler streams live execution trace events to the frontend's
// TracePanel by subscribing to the run's Redis pub/sub channel and forwarding
// every message onto the client's websocket connection.
type WebsocketHandler struct {
	Redis *redis.Client
}

func NewWebsocketHandler(rdb *redis.Client) *WebsocketHandler {
	return &WebsocketHandler{Redis: rdb}
}

// UpgradeMiddleware rejects non-websocket requests before the handshake.
func UpgradeMiddleware(c *fiber.Ctx) error {
	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}

// StreamExecution is the websocket handler itself. GET /ws/executions/:run_id
func (h *WebsocketHandler) StreamExecution() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		runID := conn.Params("run_id")
		if runID == "" {
			_ = conn.WriteJSON(fiber.Map{"type": "error", "message": "run_id is required"})
			_ = conn.Close()
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Send the currently-known run status immediately, so a client that
		// connects mid-run isn't stuck waiting for the next event.
		if status, err := redisstate.GetRunState(ctx, h.Redis, runID); err == nil && status != "" {
			_ = conn.WriteJSON(redisstate.StepEvent{
				Type: "run_status_changed", RunID: runID, Status: status, Timestamp: time.Now().UTC(),
			})
		}

		pubsub := redisstate.Subscribe(ctx, h.Redis, runID)
		defer pubsub.Close()

		msgCh := pubsub.Channel()

		// Reader goroutine: detects client disconnects (close frames, errors)
		// so we can cancel the context and stop the writer loop below.
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					cancel()
					return
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				var event redisstate.StepEvent
				if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
					log.Printf("websocket_handler: failed to unmarshal event: %v", err)
					continue
				}
				if err := conn.WriteJSON(event); err != nil {
					return
				}
				if event.Type == "run_status_changed" && (event.Status == "completed" || event.Status == "failed") {
					return
				}
			}
		}
	})
}
