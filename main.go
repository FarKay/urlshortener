package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

type shortenRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias"` // optional custom code
}

// custom aliases: 3-30 letters, digits, hyphens or underscores
var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,30}$`)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// app holds what the handlers share. They only know about the
// Store interface, not which database is behind it.
type app struct {
	store Store
}

func generateCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// storeError turns a store error into an HTTP response.
func storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "short link not found"})
		return
	}
	log.Println("store error:", err) // details stay in the server log
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong"})
}

// POST /shorten  with body {"url": "https://...", "alias": "optional"}
func (a *app) shorten(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	u, err := url.ParseRequestURI(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must be a valid http or https address"})
		return
	}

	if req.Alias != "" && !aliasPattern.MatchString(req.Alias) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "alias must be 3-30 characters: letters, digits, - or _"})
		return
	}

	link := Link{URL: req.URL, CreatedAt: time.Now()}

	if req.Alias != "" {
		// custom alias: if it is taken, tell the client
		link.Code = req.Alias
		if err := a.store.Save(r.Context(), link); err != nil {
			if errors.Is(err, ErrCodeTaken) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "alias already in use"})
				return
			}
			storeError(w, err)
			return
		}
	} else {
		// random code: just try to save, and pick a new code if it
		// is taken. The database decides, so no separate check step.
		saved := false
		for attempt := 0; attempt < 5; attempt++ {
			link.Code = generateCode(6)
			err := a.store.Save(r.Context(), link)
			if err == nil {
				saved = true
				break
			}
			if !errors.Is(err, ErrCodeTaken) {
				storeError(w, err)
				return
			}
		}
		if !saved {
			log.Println("could not find a free code after 5 attempts")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate a unique code"})
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"code":      link.Code,
		"short_url": "http://localhost:4000/" + link.Code,
	})
}

// GET /{code}  counts a click and redirects to the original URL
func (a *app) redirect(w http.ResponseWriter, r *http.Request) {
	link, err := a.store.Visit(r.Context(), mux.Vars(r)["code"])
	if err != nil {
		storeError(w, err)
		return
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
}

// GET /stats/{code}  returns the link's details and click count
func (a *app) stats(w http.ResponseWriter, r *http.Request) {
	link, err := a.store.Get(r.Context(), mux.Vars(r)["code"])
	if err != nil {
		storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// DELETE /{code}  removes a link
func (a *app) deleteLink(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Delete(r.Context(), mux.Vars(r)["code"]); err != nil {
		storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204: success, no body
}

func main() {
 	_ = godotenv.Load() // ignored if there is no .env file

	// pick the store: MongoDB if MONGO_URI is set, otherwise memory
	var store Store
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		ms, err := NewMongoStore(uri, "urlshortener", "links")
		if err != nil {
			log.Fatal("could not connect to MongoDB: ", err)
		}
		store = ms
		fmt.Println("Using MongoDB")
	} else {
		store = NewMemoryStore()
		fmt.Println("MONGO_URI not set, using the in-memory store")
	}

	// hosting platforms choose the port and pass it in PORT
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}
 
	// the public address used in short links; localhost when not set
	baseURL := strings.TrimRight(os.Getenv("BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}

	a := &app{store: store}

	r := mux.NewRouter()
	r.HandleFunc("/shorten", a.shorten).Methods("POST")
	r.HandleFunc("/stats/{code}", a.stats).Methods("GET")
	r.HandleFunc("/{code}", a.redirect).Methods("GET")
	r.HandleFunc("/{code}", a.deleteLink).Methods("DELETE")

	fmt.Println("URL shortener running on port", port, "- short links use", baseURL)
	log.Fatal(http.ListenAndServe(":"+port, r))
}