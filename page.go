package main

import "github.com/gorilla/sessions"

type Page struct {
	Session *sessions.Session
	Data    interface{}
	Err     string
}
