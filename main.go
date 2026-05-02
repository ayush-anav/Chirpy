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
	"time"

	"github.com/ayush-anav/chirpy/internal/auth"
	"github.com/ayush-anav/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // postgres driver, _ means we will use as side-effect
)

type ApiConfig struct {
	FileserverHits atomic.Int32 // allows us to increment int and read across go-routines
	// it is already 0 valid, so we can define our struct via
	// variableName := &ApiConfig
	db  *database.Queries // place where our queries live
	env string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
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
		db:  dbQueries, // now all our func can use db methods e.g cfg.db.createUser()
		env: os.Getenv("PLATFORM"),
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

	router.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		type RequestStruct struct {
			Body   string    `json:"body"`
			UserId uuid.UUID `json:"user_id"`
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
		cleanedBody := checkProfanity(reqBody.Body)

		type SuccessfulResponse struct {
			Body   string    `json:"body"`
			UserID uuid.UUID `json:"user_id"`
			ID     uuid.UUID `json:"id"`
		}

		newChirp := database.AddChirpParams{
			Body:   cleanedBody.CleanedBody,
			UserID: reqBody.UserId,
		}

		// insert resp struct to db here:
		DBRes, errAdding := cfg.db.AddChirp(r.Context(), newChirp)

		sendRespToClient := SuccessfulResponse{
			Body:   cleanedBody.CleanedBody,
			UserID: reqBody.UserId,
			ID:     DBRes.ID,
		}

		if errAdding != nil {
			respondWithError(w, 500, "Failed to save to DB!")
		}

		respondWithJSON(w, 201, sendRespToClient)
	})

	router.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		type RequestStruct struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		decoder := json.NewDecoder(r.Body)

		// CLIENT REQUEST STRUCT
		var clientRequest RequestStruct

		if err := decoder.Decode(&clientRequest); err != nil {
			respondWithError(w, 500, "Could not decode Request Body -> Struct")
			log.Printf("Error: r.Body -> Struct: %s", err)
			return
		}

		hash, errHash := auth.HashPassword(clientRequest.Password)
		if errHash != nil {
			respondWithError(w, 500, "Internal Server Error")
			log.Printf("Failed to hash password %s", errHash)
			return
		}

		newUser := database.CreateUserParams{
			Email:          clientRequest.Email,
			HashedPassword: hash,
		}

		dbUserStruct, dbErr := cfg.db.CreateUser(r.Context(), newUser)

		if dbErr != nil {
			respondWithError(w, 500, "Failed to insert to DB")
			log.Printf("Failed to insert DB %s", dbErr)
			return
		}

		resp := User{
			ID:        dbUserStruct.ID,
			CreatedAt: dbUserStruct.CreatedAt,
			UpdatedAt: dbUserStruct.UpdatedAt,
			Email:     dbUserStruct.Email,
		}

		respondWithJSON(w, 201, resp)
	})

	router.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if cfg.env != "dev" {
			respondWithError(w, 403, "Forbidden")
			return
		}

		err := cfg.db.ResetUsers(r.Context())
		if err != nil {
			respondWithError(w, 500, "failed to delete users")
			return
		}
	})

	router.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		allChirps, getErr := cfg.db.GetAllChirps(r.Context())

		if getErr != nil {
			respondWithError(w, 500, "failed to get resource")
			return
		}

		type Chirps struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Body      string    `json:"body"`
			UserID    uuid.UUID `json:"user_id"`
		}

		responseArr := []Chirps{}

		for _, chirp := range allChirps {
			serverResponse := Chirps{
				ID:        chirp.ID,
				CreatedAt: chirp.CreatedAt,
				UpdatedAt: chirp.UpdatedAt,
				Body:      chirp.Body,
				UserID:    chirp.UserID,
			}
			responseArr = append(responseArr, serverResponse)
		}

		respondWithJSON(w, 200, responseArr)
	})

	router.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		chirpString := r.PathValue("chirpID")
		chirpID, errParse := uuid.Parse(chirpString)
		if errParse != nil {
			respondWithError(w, 500, "could not gen UUID from supplied url")
			return
		}

		chirp, errGetChirpID := cfg.db.GetChripsID(r.Context(), uuid.UUID(chirpID))

		if errGetChirpID != nil {
			respondWithError(w, 404, "Resource not found!")
		}

		type Chirps struct {
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Body      string    `json:"body"`
			UserID    uuid.UUID `json:"user_id"`
		}

		serverResponse := Chirps{
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}

		respondWithJSON(w, 200, serverResponse)

	})

	router.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		// read pasword and email from r.Body
		type IncomingRequest struct {
			Password string `json:"password"`
			Email    string `json:"email"`
		}

		var userRequest IncomingRequest
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&userRequest); err != nil {
			respondWithError(w, 500, "Internal Server Error")
			log.Printf("Failed to decode req.Body -> Struct: %s", err)
			return
		}

		// get the user from sql and take the password from the user and check
		dbUser, err := cfg.db.GetUserByEmail(r.Context(), userRequest.Email)

		if err != nil {
			respondWithError(w, 500, "Internal Error")
			log.Printf("Could not fetch user from DB: %s", err)
			return
		}

		match, err := auth.CheckPasswordHash(userRequest.Password, dbUser.HashedPassword)
		if err != nil {
			respondWithError(w, 500, "Internal Match Error")
			log.Printf("Could not do ComparePasswordAndHash: %s", err)
			return
		}

		if match {
			type User struct {
				ID        uuid.UUID `json:"id"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				Email     string    `json:"email"`
			}
			sendRespToClient := User{
				ID:        dbUser.ID,
				CreatedAt: dbUser.CreatedAt,
				UpdatedAt: dbUser.UpdatedAt,
				Email:     dbUser.Email,
			}

			respondWithJSON(w, 200, sendRespToClient)
		} else {
			respondWithJSON(w, 401, "Unauthorized")
		}
	})

	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to serve: %s", err)
	}

}

type Profanity struct {
	CleanedBody string `json:"cleaned_body"`
}

func checkProfanity(body string) Profanity {

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
