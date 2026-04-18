package main

import (
	"log"
	"net/http"
)

func main() {
	router := http.NewServeMux()
	server := &http.Server{
		Handler: router,
		Addr:    ":8080",
	}

	router.Handle("/", http.FileServer(http.Dir(".")))
	router.Handle("/about", http.FileServer(http.Dir("./assets")))

	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to serve: %w", err)
	}
}
