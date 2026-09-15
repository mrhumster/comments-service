package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("JWT_ACCESS_PUBLIC_KEY_URL", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	require.Equal(t, ":8080", cfg.Server.ServerAddr)
	require.Equal(t, "debug", cfg.Server.Mode)
	require.Contains(t, cfg.Server.AllowedOrigins, "http://localhost:5173")
	require.Equal(t, "", cfg.Server.MetricsAddr)
	require.Equal(t, "localhost", cfg.Database.Host)
	require.Equal(t, "5432", cfg.Database.Port)
	require.Equal(t, "postgres", cfg.Database.Name)
	require.Equal(t, "disable", cfg.Database.SslMode)
	require.Equal(t, "UTC", cfg.Database.TimeZone)
	require.Equal(t, "localhost", cfg.Redis.Addr)
	require.Equal(t, 3, cfg.Redis.QueueDB)
}

func TestLoadConfigReadsEnv(t *testing.T) {
	t.Setenv("SERVER_ADDR", ":9090")
	t.Setenv("MODE", "release")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://a.com, https://b.com")
	t.Setenv("DB_HOST", "pg")
	t.Setenv("DB_PORT", "15432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASS", "p")
	t.Setenv("DB_NAME", "comments")
	t.Setenv("JWT_ACCESS_PUBLIC_KEY_URL", "http://identity:80/auth/public-key")
	t.Setenv("REDIS_ADDR", "redis")
	t.Setenv("REDIS_PASS", "secret")
	t.Setenv("REDIS_QUEUE_DB", "5")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	require.Equal(t, ":9090", cfg.Server.ServerAddr)
	require.Equal(t, "release", cfg.Server.Mode)
	require.Equal(t, []string{"https://a.com", "https://b.com"}, cfg.Server.AllowedOrigins)
	require.Equal(t, "pg", cfg.Database.Host)
	require.Equal(t, "15432", cfg.Database.Port)
	require.Equal(t, "u", cfg.Database.User)
	require.Equal(t, "p", cfg.Database.Password)
	require.Equal(t, "comments", cfg.Database.Name)
	require.Equal(t, "http://identity:80/auth/public-key", cfg.JWT.AccessPublicKeyURL)
	require.Equal(t, "redis", cfg.Redis.Addr)
	require.Equal(t, "secret", cfg.Redis.Password)
	require.Equal(t, 5, cfg.Redis.QueueDB)

	dsn := cfg.GetDSN()
	require.Contains(t, dsn, "host=pg")
	require.Contains(t, dsn, "dbname=comments")
	require.Contains(t, dsn, "password=p")
}

func TestLoadConfigInvalidQueueDB(t *testing.T) {
	t.Setenv("REDIS_QUEUE_DB", "abc")
	_, err := LoadConfig()
	require.Error(t, err)
}

func TestCommaSplit(t *testing.T) {
	require.Nil(t, commaSplit(""))
	require.Nil(t, commaSplit("  "))
	require.Equal(t, []string{"a", "b"}, commaSplit(" a , b "))
}
