package main

import (
	"bootdev-chirpy/internal/database"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)
import _ "github.com/lib/pq"

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricHits(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(tmpl, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) metricHitsReset(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("You are using an incorrect platform"))
		return
	}
	err := cfg.db.DeleteUsers(context.Background())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Fileserver hits reset to 0"))
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type email struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	newEmail := email{}
	err := decoder.Decode(&newEmail)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	newUser, err := cfg.db.CreateUser(r.Context(), newEmail.Email)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create user")
		return
	}
	newUserResp := User{
		ID:        newUser.ID,
		CreatedAt: newUser.CreatedAt,
		UpdatedAt: newUser.UpdatedAt,
		Email:     newUser.Email,
	}
	respondWithJSON(w, http.StatusCreated, newUserResp)
}

func main() {

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	databaseQueries, err := connectionDatabase(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	cfg := &apiConfig{
		db:       databaseQueries,
		platform: platform,
	}

	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	assetsHandler := http.StripPrefix("/app/assets/", http.FileServer(http.Dir("assets")))

	mux := http.NewServeMux()
	mux.Handle("GET /app", cfg.middlewareMetricsInc(appHandler))
	mux.Handle("GET /app/assets/", cfg.middlewareMetricsInc(assetsHandler))
	mux.HandleFunc("GET /api/healthz", readinessHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.metricHits)
	mux.HandleFunc("POST /admin/reset", cfg.metricHitsReset)
	mux.HandleFunc("POST /api/validate_chirp", validateChirpHandler)
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)

	httpServer := &http.Server{}
	httpServer.Handler = mux
	httpServer.Addr = ":8080"
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

const tmpl = `<html>
<body>
	<h1>Welcome, Chirpy Admin</h1>
	<p>Chirpy has been visited %d times!</p>
</body>
</html>`

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	respondWithJSON(w, code, errorResponse{msg})

}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	jsonResponse, err := json.Marshal(payload)
	if err != nil {
		log.Println(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(jsonResponse)
}

func validateChirpHandler(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}
	type validResponse struct {
		CleanedBody string `json:"cleaned_body"`
	}
	decoder := json.NewDecoder(r.Body)
	incomingChirp := chirp{}
	err := decoder.Decode(&incomingChirp)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}
	if utf8.RuneCountInString(incomingChirp.Body) > 140 {
		log.Printf(
			"Incoming reques chirp is too long: %d symbols",
			utf8.RuneCountInString(incomingChirp.Body),
		)
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}
	cleanedString := cleanChirp(incomingChirp.Body)
	log.Printf("Cleaned chirp for response: %s", cleanedString)
	respondWithJSON(w, http.StatusOK, validResponse{CleanedBody: cleanedString})

}

func cleanChirp(body string) string {
	notAllowedWords := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}
	splitWords := strings.Split(body, " ")
	for idx, word := range splitWords {
		if _, ok := notAllowedWords[strings.ToLower(word)]; ok {
			splitWords[idx] = "****"
		}
	}
	return strings.Join(splitWords, " ")
}

func connectionDatabase(dbURL string) (*database.Queries, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	databaseQueries := database.New(db)
	return databaseQueries, nil
}
