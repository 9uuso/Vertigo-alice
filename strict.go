package main

import (
	"net/http"
)

func StrictContentType(r *http.Request, ContentType string) bool {
	ct := r.Header.Get("Content-Type")
	if ContentType != ct {
		return false
	}
	return true
}

//application/x-www-form-urlencoded
func StrictWWWFormUrlEncoded(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := StrictContentType(r, "application/x-www-form-urlencoded")
		if !ok && r.Method != "GET" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		h.ServeHTTP(w, r)
		return
	})
}

//application/json
func StrictJSON(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := StrictContentType(r, "application/json")
		if !ok && r.Method != "GET" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		h.ServeHTTP(w, r)
		return
	})
}
