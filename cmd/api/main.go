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
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v81"
)

const Version = "1.0.0"

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
	stripe struct {
		webhookSecret string
	}
}

type Application struct {
	config     Config
	storage    *Storage
	mailer     *Mailer
	wg         sync.WaitGroup
	servicesCh chan ServiceFunc
	quit       chan struct{}
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
		config:     cfg,
		storage:    storage,
		mailer:     NewMailer(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
		servicesCh: make(chan ServiceFunc),
		quit:       make(chan struct{}),
	}

	app.Go(func() {
		log.Println("Started services manager")
	loop:
		for {
			select {
			case fn := <-app.servicesCh:
				app.launchService(fn)
			case _, open := <-app.quit:
				if !open {
					break loop
				}
			}
		}
		log.Println("Services manager was shut down gracefully")
	})

	app.StartService(app.TokensService(time.Minute))
	app.StartService(app.CheckoutSessionsService(100, time.Minute))
	app.StartService(app.TicketsService(time.Minute))

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

		close(app.quit)
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

	stripeKey := os.Getenv("STRIPE_KEY")
	if stripeKey == "" {
		return Config{}, fmt.Errorf(`environment variable "STRIPE_KEY" is not specified`)
	}

	stripeWebhook := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeWebhook == "" {
		return Config{}, fmt.Errorf(`environment variable "STRIPE_WEBHOOK_SECRET" is not specified`)
	}

	stripe.Key = stripeKey
	cfg.stripe.webhookSecret = stripeWebhook
	return cfg, nil
}
