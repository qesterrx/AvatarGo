package config

import (
	"os"
	"strconv"
)

type Config struct {
	Database DatabaseConfig
	S3       S3Config
	Rabbit   RabbitConfig
	Server   ServerConfig
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	Region          string
	UseSSL          bool
}

type RabbitConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Queue    string
}

type ServerConfig struct {
	Host string
	Port int
}

func Load() (*Config, error) {
	cfg := Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres_avatargo"),
			Password: getEnv("DB_PASSWORD", "postgres_avatargo"),
			DBName:   getEnv("DB_NAME", "avatargo"),
		},
		S3: S3Config{
			Endpoint:        getEnv("S3_ENDPOINT", "localhost:9000"),
			AccessKeyID:     getEnv("S3_ACCESS_KEY", "minio_avatargo"),
			SecretAccessKey: getEnv("S3_SECRET_KEY", "minio_avatargo"),
			BucketName:      getEnv("S3_BUCKET", "avatargo"),
			Region:          getEnv("S3_REGION", "us-east-1"),
			UseSSL:          false,
		},
		Rabbit: RabbitConfig{
			Host:     getEnv("RBT_HOST", "localhost"),
			Port:     getEnvInt("RBT_PORT", 5672),
			User:     getEnv("RBT_USER", "rabbitmq_avatargo"),
			Password: getEnv("RBT_PASSWORD", "rabbitmq_avatargo"),
			Queue:    getEnv("RBT_QUEUE", "avatargo"),
		},
		Server: ServerConfig{
			Host: getEnv("SRV_HOST", ""),
			Port: getEnvInt("SRV_PORT", 8080),
		},
	}

	return &cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
