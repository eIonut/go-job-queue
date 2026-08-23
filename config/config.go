package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	DatabaseURL  string
	WorkerCount  int
	MaxAttempts  int
	PollInterval time.Duration
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "5001"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5433/jobqueue?sslmode=disable"),
		WorkerCount:  getEnvInt("WORKER_COUNT", 5),
		MaxAttempts:  getEnvInt("MAX_ATTEMPTS", 3),
		PollInterval: time.Duration(getEnvInt("POLL_INTERVAL_SECONDS", 1)) * time.Second,
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
