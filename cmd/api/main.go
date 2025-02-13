package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const Version = "1.0.0"

type Config struct {
	port        int
	environment string
	db          struct {
		dsn string
	}
}

type Application struct {
	config  Config
	storage *Storage
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	queryTimeout := 5 * time.Second
	storage, err := NewStorage(cfg.db.dsn, queryTimeout)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connected to database")

	app := &Application{
		config:  cfg,
		storage: storage,
	}

	addr := fmt.Sprintf(":%d", cfg.port)
	srv := http.Server{
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		Addr:         addr,
		Handler:      composeRoutes(app),
	}

	quit := make(chan error)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
		<-sig

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		log.Println("Starting server shutdown")
		err := srv.Shutdown(ctx)

		quit <- err
	}()

	log.Printf("Starting server on port %d\n", cfg.port)
	err = srv.ListenAndServe()
	if err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server wasn't shutdown gracefully: %v\n", err)
		}
	}
	err = <-quit
	if err != nil {
		log.Fatalf("Server wasn't shutdown gracefully: %v\n", err)
	}
	log.Println("Server was shutdown gracefully")
}

func loadConfig() (Config, error) {
	err := godotenv.Load()
	if err != nil {
		return Config{}, fmt.Errorf("failed to load configuration: %w", err)
	}

	env := os.Getenv("ENV")

	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return Config{}, fmt.Errorf(`invalid environment variable "PORT" in configuration: %w`, err)
	}

	cfg := Config{
		environment: env,
		port:        port,
	}

	cfg.db.dsn = os.Getenv("DB_DSN")
	if cfg.db.dsn == "" {
		return Config{}, fmt.Errorf(`environment variable "DB_DSN" is not specified`)
	}
	return cfg, nil
}
