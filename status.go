package main

import (
	"net/http"
)

func handleStatus() http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}

	return http.HandlerFunc(f)
}
