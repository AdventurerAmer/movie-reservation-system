package main

import (
	"bytes"
	"crypto/tls"
	"html/template"
	"log"

	gomail "gopkg.in/mail.v2"
)

type Mailer struct {
	dailer *gomail.Dialer
	sender string
}

func NewMailer(host string, port int, username, password, sender string) *Mailer {
	d := gomail.NewDialer(host, port, username, password)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	return &Mailer{
		dailer: d,
		sender: sender,
	}
}

func (m *Mailer) Send(to string, tmpl *template.Template, data any) error {
	var subject bytes.Buffer
	err := tmpl.ExecuteTemplate(&subject, "subject", data)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	err = tmpl.ExecuteTemplate(&body, "body", data)
	if err != nil {
		return err
	}
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.sender)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject.String())
	msg.SetBody("text/html", body.String())
	for i := 0; i < 3; i++ {
		err = m.dailer.DialAndSend(msg)
		if nil == err {
			break
		}
		log.Println(err)
	}
	return err
}
