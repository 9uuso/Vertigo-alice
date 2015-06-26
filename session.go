package main

import (
	"log"
	"net/http"

	"github.com/gorilla/sessions"
)

var store = sessions.NewCookieStore([]byte(Settings.CookieHash))

const SESSIONNAME string = "vertigosession"

func SessionInit(r *http.Request) {
	session, _ := store.Get(r, SESSIONNAME)
	log.Println(session)
	//session.Options = &sessions.Options{Path: "/", Secure: true, HttpOnly: true}
}

func SessionGetValue(r *http.Request, key string) (value int64, ok bool) {
	session, _ := store.Get(r, SESSIONNAME)
	//log.Printf("\n---------\nstore get options:\n%+v\n", session.Options)
	//log.Printf("\n---------\nstore get values:\n%+v\n", session.Values)
	token, ok := session.Values[key]
	if ok {
		return token.(int64), true
	}
	return -1, false
}

func SessionSetValue(w http.ResponseWriter, r *http.Request, key string, value int64) {
	session, _ := store.Get(r, SESSIONNAME)
	// Set some session values.
	session.Values[key] = value
	//log.Println("key: ", key, " value: ", value)
	//log.Printf("\n---------\nstore set options:\n%+v\n", session.Options)
	//log.Printf("\n---------\nstore set values:\n%+v\n", session.Values)
	// Save it.
	session.Save(r, w)
}

func SessionDelete(w http.ResponseWriter, r *http.Request, key string) {
	//log.Println("deleting session")
	session, _ := store.Get(r, SESSIONNAME)
	session.Options = &sessions.Options{Path: "/", MaxAge: -1, Secure: true, HttpOnly: true}
	session.Save(r, w)
}

// SessionRedirect in addition to sessionIsAlive makes HTTP redirection to user home.
// SessionRedirect is useful for redirecting from pages which are only visible when logged out,
// for example login and register pages.
/*
type SessionRedirect struct {
	middle bool
}

func NewSessionRedirect() *SessionRedirect {
	return &SessionRedirect{true}
}
func (session SessionRedirect) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if SessionIsAlive(r) {
		http.Redirect(w, r, "/user", http.StatusFound)
	}
	next(w, r)
}
*/

func SessionRedirect(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SessionIsAlive(r) {
			http.Redirect(w, r, "/user", http.StatusFound)
		}
		h.ServeHTTP(w, r)
		return
	})
}

func SessionIsAlive(r *http.Request) bool {
	s, ok := SessionGetValue(r, "id")
	//log.Println("session: ", s)
	if s < 1 || ok == false {
		return false
	}
	return true
}

func ProtectedPage(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !SessionIsAlive(r) {
			SessionDelete(w, r, "id")
			rend.JSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Unauthorized"})
			return
		}
		h.ServeHTTP(w, r)
		return
	})
}
