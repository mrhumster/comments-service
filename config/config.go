package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server   Server
	Database Database
	JWT      JWT
	Redis    Redis
}

type Server struct {
	ServerAddr     string
	Mode           string
	AllowedOrigins []string
	MetricsAddr    string
}

type Database struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SslMode  string
	TimeZone string
}

type JWT struct {
	AccessPublicKeyURL string
}

// Redis is used only as an asynq producer client for the events-service
// queue; comments themselves are served synchronously from Postgres.
type Redis struct {
	Addr     string
	Password string
	QueueDB  int
}

func LoadConfig() (*Config, error) {
	queueDB, err := strconv.ParseUint(getEnv("REDIS_QUEUE_DB", "3"), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("events queue DB: parse REDIS_QUEUE_DB: %w", err)
	}

	return &Config{
		Database: Database{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASS", ""),
			Name:     getEnv("DB_NAME", "postgres"),
			SslMode:  "disable",
			TimeZone: "UTC",
		},
		Server: Server{
			ServerAddr:     getEnv("SERVER_ADDR", ":8080"),
			Mode:           getEnv("MODE", "debug"),
			AllowedOrigins: commaSplit(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173,https://example.com,https://comments.example.com")),
			MetricsAddr:    getEnv("METRICS_ADDR", ""),
		},
		JWT: JWT{
			AccessPublicKeyURL: os.Getenv("JWT_ACCESS_PUBLIC_KEY_URL"),
		},
		Redis: Redis{
			Addr:     getEnv("REDIS_ADDR", "localhost"),
			Password: getEnv("REDIS_PASS", ""),
			QueueDB:  int(queueDB),
		},
	}, nil
}

func (c *Config) GetDSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.User,
		c.Database.Password,
		c.Database.Name,
		c.Database.SslMode,
		c.Database.TimeZone)
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func commaSplit(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
