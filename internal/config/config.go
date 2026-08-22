package config

import (
	"fmt"
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
		Database: DatabaseConfig{},
		S3:       S3Config{},
		Rabbit:   RabbitConfig{},
		Server:   ServerConfig{},
	}

	var exists bool

	//DATABASE CONFIG

	cfg.Database.Host = getEnv("DB_HOST", "")
	cfg.Database.Port = getEnvInt("DB_PORT", 5432)
	cfg.Database.DBName = getEnv("DB_HOST", "avatargo")

	cfg.Database.User, exists = os.LookupEnv("DB_USER")
	if !exists {
		return nil, fmt.Errorf("ENV DB_USER undefined")
	}
	cfg.Database.Password, exists = os.LookupEnv("DB_PASSWORD")
	if !exists {
		return nil, fmt.Errorf("ENV DB_PASSWORD undefined")
	}

	//RABBIT CONFIG

	cfg.Rabbit.Host = getEnv("RBT_HOST", "")
	cfg.Rabbit.Port = getEnvInt("RBT_PORT", 5672)
	cfg.Rabbit.Queue = getEnv("RBT_HOST", "avatargo")

	cfg.Rabbit.User, exists = os.LookupEnv("RBT_USER")
	if !exists {
		return nil, fmt.Errorf("ENV RBT_USER undefined")
	}
	cfg.Rabbit.Password, exists = os.LookupEnv("RBT_PASSWORD")
	if !exists {
		return nil, fmt.Errorf("ENV RBT_PASSWORD undefined")
	}

	//S3 CONFIG

	cfg.S3.Endpoint = getEnv("S3_ENDPOINT", ":9000")
	cfg.S3.BucketName = getEnv("S3_BUCKET", "avatargo")
	cfg.S3.Region = getEnv("S3_REGION", "us-east-1")

	cfg.S3.AccessKeyID, exists = os.LookupEnv("S3_ACCESS_KEY")
	if !exists {
		return nil, fmt.Errorf("ENV S3_ACCESS_KEY undefined")
	}
	cfg.S3.SecretAccessKey, exists = os.LookupEnv("S3_SECRET_KEY")
	if !exists {
		return nil, fmt.Errorf("ENV S3_SECRET_KEY undefined")
	}

	//SERVER CONFIG

	cfg.Server.Host = getEnv("SRV_HOST", "")
	cfg.Server.Port = getEnvInt("SRV_PORT", 8080)

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
