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

	router.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))
	router.Handle("/app/assets", http.StripPrefix("/app/assets", http.FileServer(http.Dir("./assets"))))

	router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to serve: %w", err)
	}
}
