package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)
import _ "github.com/lib/pq"

type apiConfig struct {
	fileserverHits atomic.Int32
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
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Fileserver hits reset to 0"))
}

func main() {
	cfg := &apiConfig{}
	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	assetsHandler := http.StripPrefix("/app/assets/", http.FileServer(http.Dir("assets")))

	mux := http.NewServeMux()
	mux.Handle("GET /app", cfg.middlewareMetricsInc(appHandler))
	mux.Handle("GET /app/assets/", cfg.middlewareMetricsInc(assetsHandler))
	mux.HandleFunc("GET /api/healthz", readinessHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.metricHits)
	mux.HandleFunc("POST /admin/reset", cfg.metricHitsReset)
	mux.HandleFunc("POST /api/validate_chirp", validateChirpHandler)

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
