package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
	"github.com/stripe/stripe-go/v81"
)

//	@title			Movie Reservation System API
//	@version		1.0
//	@description	a simple movie reservation system api for educational purposes

//	@contact.name	Ahmed Amer
//	@contact.email	ahamerdev@gmail.com

//	@host		https://localhost:8080
//	@BasePath	/v1

const Version = "1.0.0"

type Application struct {
	config     Config
	storage    *internal.Storage
	mailer     *Mailer
	wg         sync.WaitGroup
	servicesCh chan ServiceFunc
	quit       chan struct{}
}

//go:embed templates
var Templates embed.FS
var ActivateUserTmpl *template.Template
var ResetPasswordTempl *template.Template

func init() {
	var err error
	ActivateUserTmpl, err = template.ParseFS(Templates, "templates/activate_user.gotmpl")
	if err != nil {
		panic(err)
	}
	ResetPasswordTempl, err = template.ParseFS(Templates, "templates/reset_password.gotmpl")
	if err != nil {
		panic(err)
	}
}

func main() {
	log.SetFlags(log.LUTC | log.Llongfile)

	cfg := MustLoadConfig()
	stripe.Key = cfg.stripe.key

	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		log.Fatal(err)
	}

	db.SetMaxIdleConns(cfg.db.maxIdleConns)
	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetConnMaxIdleTime(cfg.db.maxConnIdelTime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connected to database")

	app := &Application{
		config:     *cfg,
		storage:    internal.NewStorage(db, cfg.db.queryTimeout),
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

	tlsConfig := &tls.Config{
		MinVersion:       tls.VersionTLS12,
		MaxVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	addr := fmt.Sprintf(":%d", cfg.port)
	srv := http.Server{
		TLSConfig:    tlsConfig,
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
	err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")
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
