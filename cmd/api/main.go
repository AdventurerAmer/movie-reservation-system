package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const Version = "1.0.0"

//go:embed templates
var Templates embed.FS

type Config struct {
	port        int
	environment string
	db          struct {
		dsn string
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
}

type Application struct {
	config  Config
	storage *Storage
	mailer  *Mailer
	wg      sync.WaitGroup
}

func (app *Application) Go(fn func()) {
	app.wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Println(err)
			}
			app.wg.Done()
		}()
		fn()
	}()
}

func main() {
	log.SetFlags(log.LUTC | log.Llongfile)

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
		mailer:  NewMailer(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
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

		log.Println("Waiting for background goroutines")
		app.wg.Wait()

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

	port, err := strconv.Atoi(os.Getenv("SERVER_PORT"))
	if err != nil {
		return Config{}, fmt.Errorf(`invalid environment variable "SERVER_PORT" in configuration: %w`, err)
	}

	cfg := Config{
		environment: env,
		port:        port,
	}

	cfg.db.dsn = os.Getenv("DB_DSN")
	if cfg.db.dsn == "" {
		return Config{}, fmt.Errorf(`environment variable "DB_DSN" is not specified`)
	}

	cfg.smtp.host = os.Getenv("SMTP_HOST")
	if cfg.smtp.host == "" {
		return Config{}, fmt.Errorf(`environment variable "SMTP_HOST" is not specified`)
	}

	port, err = strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		return Config{}, fmt.Errorf(`invalid environment variable "SMTP_PORT" in configuration: %w`, err)
	}
	cfg.smtp.port = port

	cfg.smtp.username = os.Getenv("SMTP_USERNAME")
	if cfg.smtp.username == "" {
		return Config{}, fmt.Errorf(`environment variable "SMTP_USERNAME" is not specified`)
	}
	cfg.smtp.password = os.Getenv("SMTP_PASSWORD")
	if cfg.smtp.password == "" {
		return Config{}, fmt.Errorf(`environment variable "SMTP_PASSWORD" is not specified`)
	}
	cfg.smtp.sender = os.Getenv("SMTP_SENDER")
	if cfg.smtp.sender == "" {
		return Config{}, fmt.Errorf(`environment variable "SMTP_SENDER" is not specified`)
	}
	return cfg, nil
}
