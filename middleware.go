package main

import (
	"log"
	"net/http"

	"github.com/gorilla/context"
)

// MiddleWare
type MiddleWareA struct {
	middle bool
}

func NewMiddlewareA() *MiddleWareA {
	return &MiddleWareA{true}
}
func (m *MiddleWareA) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	var vals string

	// Call the next middleware handler
	val, ok := context.GetOk(r, MyKey)
	if !ok {
		log.Println("ok a-1: ", ok)
	} else {
		vals = val.(string)
	}
	log.Println("pre servehttp middle a, context: ", vals)
	context.Set(r, MyKey, "middle a")
	next(w, r)
	val, ok = context.GetOk(r, MyKey)
	if !ok {
		log.Println("ok a-2: ", ok)
	} else {
		vals = val.(string)
	}
	log.Println("post servehttp middle a, context: ", vals)
}

// MiddleWareB
type MiddleWareB struct {
	middle bool
}

func NewMiddlewareB() *MiddleWareB {
	return &MiddleWareB{true}
}
func (m *MiddleWareB) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	var vals string

	// Call the next middleware handler
	val, ok := context.GetOk(r, MyKey)
	if !ok {
		log.Println("ok b-1: ", ok)
	} else {
		vals = val.(string)
	}
	log.Println("pre servehttp middle b, context: ", vals)
	context.Set(r, MyKey, "middle b")
	next(w, r)
	val, ok = context.GetOk(r, MyKey)
	if !ok {
		log.Println("ok b-2: ", ok)
	} else {
		vals = val.(string)
	}
	log.Println("post servehttp middle b, context: ", vals)
}
