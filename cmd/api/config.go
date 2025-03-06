package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	port        int
	environment string
	db          struct {
		dsn             string
		maxIdleConns    int
		maxOpenConns    int
		maxConnIdelTime time.Duration
		queryTimeout    time.Duration
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
	limiter struct {
		maxRequestPerSecond float64
		burst               int
	}
	cors struct {
		trustedOrigins []string
	}
	stripe struct {
		key           string
		webhookSecret string
	}
}

func MustLoadConfig() *Config {
	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
		}
	}()

	err := godotenv.Load()
	if err != nil {
		panic(fmt.Errorf("failed to load configuration: %w", err))
	}

	cfg := Config{}

	cfg.environment = MustGetStringEnvVar("ENV")
	cfg.port = MustGetIntEnvVar("PORT")

	cfg.db.dsn = MustGetStringEnvVar("DB_DSN")
	cfg.db.maxIdleConns = MustGetIntEnvVar("DB_MAX_IDEL_CONNS")
	cfg.db.maxOpenConns = MustGetIntEnvVar("DB_MAX_OPEN_CONNS")
	cfg.db.maxConnIdelTime = MustGetDureationEnvVar("DB_MAX_CONN_IDEL_TIME")
	cfg.db.queryTimeout = MustGetDureationEnvVar("DB_QUERY_TIMEOUT")

	cfg.smtp.host = MustGetStringEnvVar("SMTP_HOST")
	cfg.smtp.port = MustGetIntEnvVar("SMTP_PORT")
	cfg.smtp.username = MustGetStringEnvVar("SMTP_USERNAME")
	cfg.smtp.password = MustGetStringEnvVar("SMTP_PASSWORD")
	cfg.smtp.sender = MustGetStringEnvVar("SMTP_SENDER")

	cfg.limiter.maxRequestPerSecond = MustGetFloatEnvVar("LIMITER_MAX_RPS")
	cfg.limiter.burst = MustGetIntEnvVar("LIMITER_BURST")

	cfg.cors.trustedOrigins = strings.Fields(MustGetStringEnvVar("CORS_TRUSTED_ORIGINS"))

	cfg.stripe.key = MustGetStringEnvVar("STRIPE_KEY")
	cfg.stripe.webhookSecret = MustGetStringEnvVar("STRIPE_WEBHOOK_SECRET")

	return &cfg
}

func MustGetStringEnvVar(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf(`environment variable "%s" is not specified`, key))
	}
	return value
}

func MustGetIntEnvVar(key string) int {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf(`environment variable "%s" is not specified`, key))
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Errorf(`environment variable "%s" is not valid int: %w`, key, err))
	}
	return n
}

func MustGetFloatEnvVar(key string) float64 {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf(`environment variable "%s" is not specified`, key))
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		panic(fmt.Errorf(`environment variable "%s" is not valid float: %w`, key, err))
	}
	return n
}

func MustGetDureationEnvVar(key string) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf(`environment variable "%s" is not specified`, key))
	}
	n, err := time.ParseDuration(value)
	if err != nil {
		panic(fmt.Errorf(`environment variable "%s" is not valid duration: %w`, key, err))
	}
	return n
}
