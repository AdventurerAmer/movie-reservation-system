package main

import (
	"html/template"
	"log"
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
