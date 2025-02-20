package main

import (
	"html/template"
	"log"
	"time"

	"github.com/stripe/stripe-go/v81/checkout/session"
)

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

func (app *Application) SendMail(to string, tmpl *template.Template, data any) func() {
	return func() {
		app.mailer.Send(to, tmpl, data)
	}
}

type ServiceFunc func()

func (app *Application) launchService(fn ServiceFunc) {
	app.wg.Add(1)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Println(err)
				app.servicesCh <- fn
			}
		}()
		app.wg.Done()
		fn()
	}()
}

func (app *Application) StartService(fn ServiceFunc) {
	app.servicesCh <- fn
}

func (app *Application) TokensService(tickRate time.Duration) ServiceFunc {
	return func() {
		log.Println("Started tokens background service")
		ticker := time.NewTicker(tickRate)
	loop:
		for {
			select {
			case <-ticker.C:
				n, err := app.storage.DeleteAllExpiredTokens()
				if err != nil {
					log.Println(err)
				} else if n != 0 {
					log.Printf("Deleted %d tokens\n", n)
				}
			case _, open := <-app.quit:
				if !open {
					break loop
				}
			}
		}
		log.Println("Tokens service was shut down gracefully")
	}
}

func (app *Application) CheckoutSessionsService(checkoutSessionsPullCount int, tickRate time.Duration) ServiceFunc {
	return func() {
		log.Println("Started checkout sessions service")
		ticker := time.NewTicker(tickRate)
	loop:
		for {
			select {
			case <-ticker.C:
				checkoutSessions, err := app.storage.GetExpiredCheckoutSessions(int64(checkoutSessionsPullCount))
				if err != nil {
					log.Println(err)
					break
				}
				for _, cs := range checkoutSessions {
					_, err := session.Expire(cs.SessionID, nil)
					if err != nil {
						log.Printf("%T %v\n", err, err)
					}
					err = app.storage.DeleteUserCheckoutSession(cs.UserID)
					if err != nil {
						log.Println(err)
					}
				}
			case _, open := <-app.quit:
				if !open {
					break loop
				}
			}
		}
		log.Println("Checkout sessions service was shut down gracefully")
	}
}

func (app *Application) TicketsService(tickRate time.Duration) ServiceFunc {
	return func() {
		log.Println("Started tickets service")
		ticker := time.NewTicker(tickRate)
	loop:
		for {
			select {
			case <-ticker.C:
				n, err := app.storage.UnlockExpiredTickets()
				if err != nil {
					log.Println(err)
					break
				}
				log.Printf("Unlocked %d tickets\n", n)
			case _, open := <-app.quit:
				if !open {
					break loop
				}
			}
		}
		log.Println("Tickets service was shut down gracefully")
	}
}
