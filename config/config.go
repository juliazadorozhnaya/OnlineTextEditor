package config

import (
	"os"
)

// AppConfig holds all configuration for the application
type AppConfig struct {
	ServerPort string
	JWTSecret  string
	DataPath   string
}

// LoadConfig loads configuration settings from environment variables or default values
func LoadConfig() *AppConfig {
	return &AppConfig{
		ServerPort: getEnv("SERVER_PORT", "8080"),
		JWTSecret:  getEnv("JWT_SECRET", "my_secret_key"),
		DataPath:   getEnv("DATA_PATH", "./data"),
	}
}

// getEnv helps get an environment variable or return a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
