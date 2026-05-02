package main

import (
	"bootdev-chirpy/internal/auth"
	"bootdev-chirpy/internal/database"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)
import _ "github.com/lib/pq"

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	jwtSecret      string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
}

type Chirp struct {
	Body string `json:"body"`
}

type ChirpResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
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
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	newRequest := request{}
	err := decoder.Decode(&newRequest)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	_, err = cfg.usersExists(r.Context(), newRequest.Email)
	if err == nil {
		respondWithError(w, http.StatusConflict, "Email already registered")
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		respondWithError(w, http.StatusInternalServerError, "Could not create user")
		return
	}
	hashedPassword, err := auth.HashPassword(newRequest.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create user")
		return
	}
	newUser, err := cfg.db.CreateUser(
		r.Context(),
		database.CreateUserParams{
			Email:          newRequest.Email,
			HashedPassword: hashedPassword,
		},
	)
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

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type loginRequest struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		ExpiresInSec int    `json:"expires_in_sec"`
	}
	decoder := json.NewDecoder(r.Body)
	newRequest := loginRequest{}
	err := decoder.Decode(&newRequest)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	existingUser, err := cfg.usersExists(r.Context(), newRequest.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "User not found")
		return
	}
	validatePassword, err := auth.CheckPasswordHash(newRequest.Password, existingUser.HashedPassword)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid password")
		return
	}
	if !validatePassword {
		respondWithError(w, http.StatusUnauthorized, "Invalid password")
		return
	}
	expires := newRequest.ExpiresInSec
	if expires <= 0 || expires > 3600 {
		expires = 3600 // default: 1 hour
	}
	token, err := auth.MakeJWT(existingUser.ID, cfg.jwtSecret, time.Duration(expires)*time.Second)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create token")
		return
	}
	userResp := User{
		ID:        existingUser.ID,
		CreatedAt: existingUser.CreatedAt,
		UpdatedAt: existingUser.UpdatedAt,
		Email:     existingUser.Email,
		Token:     token,
	}
	respondWithJSON(w, http.StatusOK, userResp)
}

func (cfg *apiConfig) createChirpHandler(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
	}
	decoder := json.NewDecoder(r.Body)
	var newChirp Chirp
	err = decoder.Decode(&newChirp)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	formatBody, err := validateChirpHandler(newChirp.Body)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	newChirp.Body = formatBody
	createChirp := database.CreateChirpParams{UserID: userID, Body: newChirp.Body}
	createdChirp, err := cfg.db.CreateChirp(r.Context(), createChirp)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create chirp")
		return
	}
	responseChirp := ChirpResponse{
		ID:        createdChirp.ID,
		CreatedAt: createdChirp.CreatedAt,
		UpdatedAt: createdChirp.UpdatedAt,
		Body:      createdChirp.Body,
		UserID:    createdChirp.UserID,
	}
	respondWithJSON(w, http.StatusCreated, responseChirp)
}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	allChirps, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get chirps")
		return
	}

	respChirps := make([]ChirpResponse, 0, len(allChirps))
	for _, chirp := range allChirps {
		respChirp := ChirpResponse{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}
		respChirps = append(respChirps, respChirp)
	}
	respondWithJSON(w, http.StatusOK, respChirps)
}

func (cfg *apiConfig) getChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request  for chirp")
		return
	}
	chirp, err := cfg.db.GetChirp(r.Context(), chirpUUID)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithError(w, http.StatusNotFound, "Chirp not found")
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get chirp")
		return
	}
	responseChirp := ChirpResponse{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	respondWithJSON(w, http.StatusOK, responseChirp)
}

func (cfg *apiConfig) usersExists(ctx context.Context, email string) (database.User, error) {
	registeredUser, err := cfg.db.GetUserByEmail(ctx, email)
	if err != nil {
		return database.User{}, err
	}
	return registeredUser, nil
}

func main() {

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	jwtSecret := os.Getenv("JWT_SECRET")
	databaseQueries, err := connectionDatabase(dbURL)
	if err != nil {
		log.Fatal(err)
	}
	cfg := &apiConfig{
		db:        databaseQueries,
		platform:  platform,
		jwtSecret: jwtSecret,
	}

	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	assetsHandler := http.StripPrefix("/app/assets/", http.FileServer(http.Dir("assets")))

	mux := http.NewServeMux()
	mux.Handle("GET /app", cfg.middlewareMetricsInc(appHandler))
	mux.Handle("GET /app/assets/", cfg.middlewareMetricsInc(assetsHandler))

	mux.HandleFunc("GET /api/chirps", cfg.getChirpsHandler)
	mux.HandleFunc("POST /api/chirps", cfg.createChirpHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirpHandler)
	mux.HandleFunc("GET /api/healthz", readinessHandler)
	mux.HandleFunc("POST /api/login", cfg.loginHandler)
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)

	mux.HandleFunc("GET /admin/metrics", cfg.metricHits)
	mux.HandleFunc("POST /admin/reset", cfg.metricHitsReset)

	httpServer := &http.Server{}
	httpServer.Handler = mux
	httpServer.Addr = ":8080"

	signalChan := make(chan os.Signal, 1)
	errChan := make(chan error)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	fmt.Printf("Starting server at %s\n", httpServer.Addr)
	go func() {
		errChan <- httpServer.ListenAndServe()
	}()
	select {
	case <-signalChan:
		fmt.Println("Server is shutting down...")
	case err := <-errChan:
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

func validateChirpHandler(body string) (string, error) {
	if utf8.RuneCountInString(body) > 140 {
		log.Printf(
			"Incoming chirp is too long: %d symbols",
			utf8.RuneCountInString(body),
		)
		return "", errors.New("chirp is too long")
	}
	cleanBody := cleanChirp(body)
	log.Printf("Cleaned chirp for response: %s", cleanBody)
	return cleanBody, nil
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
