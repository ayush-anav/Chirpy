package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/ayush-anav/chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // postgres driver, _ means we will use as side-effect
)

type ApiConfig struct {
	FileserverHits atomic.Int32 // allows us to increment int and read across go-routines
	// it is already 0 valid, so we can define our struct via
	// variableName := &ApiConfig
	db *database.Queries // place where our queries live
}

func (cfg *ApiConfig) MiddlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.FileserverHits.Add(1)
		w.WriteHeader(200)
		next.ServeHTTP(w, r)
	})
}

func main() {
	// load our fken env file
	godotenv.Load()

	// get the URL
	dbURL := os.Getenv("DB_URL")

	// setup connection to DB
	db, dbErr := sql.Open("postgres", dbURL)

	if dbErr != nil {
		log.Fatalf("Could not setup connection to DB")
	}

	// hookup db to database from our sqlc to get
	// access to queries
	dbQueries := database.New(db)

	cfg := &ApiConfig{
		db: dbQueries, // now all our func can use db methods e.g cfg.db.createUser()
	}

	router := http.NewServeMux()
	server := &http.Server{
		Handler: router,
		Addr:    ":8080",
	}
	// static file serving
	router.Handle("/app/", http.StripPrefix("/app/", cfg.MiddlewareMetricsInc(http.FileServer(http.Dir(".")))))
	router.Handle("/app/assets", http.StripPrefix("/app/assets", cfg.MiddlewareMetricsInc(http.FileServer(http.Dir("./assets")))))

	router.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	router.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<html>
										<body>
											<h1>Welcome, Chirpy Admin</h1>
											<p>Chirpy has been visited %d times!</p>
										</body>
									</html>`,
			cfg.FileserverHits.Load())))
	})

	router.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		cfg.FileserverHits.Store(0)
	})

	router.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		// take the incoming body
		// if len > 140, respond with err msg which is JSON body of this shape {"error": "Chirp is too long"}
		// same story for other err we check
		// respondWithError(w, CODE, msg)
		type RequestStruct struct {
			Body string `json:"body"`
		}

		decoder := json.NewDecoder(r.Body)
		var reqBody RequestStruct
		// failed to decode, server err
		if err := decoder.Decode(&reqBody); err != nil {
			respondWithError(w, 500, "Something went wrong")
			return
		}
		// chirp too long
		if len(reqBody.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}

		// check profanity and pass the struct that you want respondwithjson to use
		respStruct := checkProfanity(reqBody.Body)
		respondWithJSON(w, 200, respStruct)
	})

	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to serve: %w", err)
	}
}

func checkProfanity(body string) interface{} {
	type Profanity struct {
		CleanedBody string `json:"cleaned_body"`
	}

	badWords := []string{"kerfuffle", "sharbert", "fornax"}
	reqSlice := strings.Split(body, " ")
	// badWordPresent := false

	for i, reqWord := range reqSlice {
		for _, badWord := range badWords {
			if strings.EqualFold(reqWord, badWord) {
				reqSlice[i] = "****"
				// badWordPresent = true
			}
		}
	}

	profanityExists := Profanity{CleanedBody: strings.Join(reqSlice, " ")}
	return profanityExists

}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type ErrorMsg struct {
		Error string `json:"error"` // if we dont use this JSON, the resp body will look like what we name our key
	}
	error := ErrorMsg{Error: msg}
	data, err := json.Marshal(error)

	// this is our server error if we fail to marshal
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithJSON(w http.ResponseWriter, code int, respStruct interface{}) {
	// interface{} = empty struct

	data, err := json.Marshal(respStruct)
	// if marshal is unsuccessful
	if err != nil {
		w.WriteHeader(500)
		log.Printf("Failed to marshal outgoing payload: %s", err)
		return
	}

	// send the payload off to the user
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
