package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all environment-driven settings for the go-engine service.
type Config struct {
	Port              string
	Env               string
	DatabaseURL       string
	RedisURL          string
	PyPlannerURL      string
	InternalAPISecret string
}

// Load reads a .env file (if present) and environment variables into a Config.
// Environment variables always take precedence over .env file values.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("config: no .env file found, relying on process environment")
	}

	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		Env:               getEnv("ENV", "development"),
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		RedisURL:          getEnv("REDIS_URL", "localhost:6379"),
		PyPlannerURL:      getEnv("PY_PLANNER_URL", "http://localhost:8000"),
		InternalAPISecret: getEnv("INTERNAL_API_SECRET", ""),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("config: DATABASE_URL is required but not set")
	}
	if cfg.InternalAPISecret == "" {
		log.Println("config: WARNING - INTERNAL_API_SECRET is empty; auth middleware will reject all requests")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}