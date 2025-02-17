package main

import (
	"html/template"
	"log"
	"time"
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
