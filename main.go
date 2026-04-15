package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		log.Fatalf("failed to serve")
	}
}
