package main

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"github.com/MuaazTasawar/integradock/go-engine/internal/config"
	"github.com/MuaazTasawar/integradock/go-engine/internal/db"
	"github.com/MuaazTasawar/integradock/go-engine/internal/routes"
)

func main() {
	ctx := context.Background()

	cfg := config.Load()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("main: failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("main: failed to connect to redis: %v", err)
	}
	log.Println("main: connected to redis successfully")

	app := fiber.New(fiber.Config{
		AppName: "IntegraDock Go Engine",
	})

	routes.Register(app, routes.Deps{
		DB:                pool,
		Redis:             redisClient,
		InternalAPISecret: cfg.InternalAPISecret,
	})

	log.Printf("main: starting server on port %s (env=%s)", cfg.Port, cfg.Env)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("main: server stopped: %v", err)
	}
}
