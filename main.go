package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type ApiConfig struct {
	FileserverHits atomic.Int32 // allows us to increment int and read across go-routines
	// it is already 0 valid, so we can define our struct via
	// variableName := &ApiConfig
}

func (cfg *ApiConfig) MiddlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.FileserverHits.Add(1)
		w.WriteHeader(200)
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfg := &ApiConfig{}

	router := http.NewServeMux()
	server := &http.Server{
		Handler: router,
		Addr:    ":8080",
	}

	router.Handle("/app/", http.StripPrefix("/app/", cfg.MiddlewareMetricsInc(http.FileServer(http.Dir(".")))))
	router.Handle("/app/assets", http.StripPrefix("/app/assets", cfg.MiddlewareMetricsInc(http.FileServer(http.Dir("./assets")))))

	router.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	router.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(fmt.Sprintf("Hits: %v", cfg.FileserverHits.Load())))
	})

	router.HandleFunc("POST /reset", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		cfg.FileserverHits.Store(0)
	})
	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to serve: %w", err)
	}
}
