package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Redis    RedisConfig
	RedisPsn RedisConfig
	DB       DBConfig
	Auth     AuthConfig
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

type AuthConfig struct {
	ServiceURL string
}

func LoadConfig() Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	redisPSNDB, _ := strconv.Atoi(getEnv("REDIS_PSN_DB", "0"))

	return Config{
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		RedisPsn: RedisConfig{
			Host:     getEnv("REDIS_PSN_HOST", "localhost"),
			Port:     getEnv("REDIS_PSN_PORT", "6380"),
			Password: getEnv("REDIS_PSN_PASSWORD", ""),
			DB:       redisPSNDB,
		},
		Auth: AuthConfig{
			ServiceURL: getEnv("AUTH_SERVICE_URL", "localhost:50051"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
