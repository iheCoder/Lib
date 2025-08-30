package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type sessionRequest struct {
	Model      string   `json:"model"`
	Voice      string   `json:"voice,omitempty"`
	Modalities []string `json:"modalities,omitempty"`
	// You can add more fields as needed, e.g. Instructions, etc.
}

func main() {
	addr := getenv("PORT", "8080")
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Println("[WARN] OPENAI_API_KEY not set. /session will fail until provided.")
	}
	model := getenv("REALTIME_MODEL", "gpt-4o-realtime-preview-2024-12-17")
	voice := os.Getenv("VOICE") // optional, e.g. "verse" or "alloy"
	// Default to current directory so `cd llm/realtime/fe && go run ./server` works.
	feDir := getenv("FE_DIR", ".")

	mux := http.NewServeMux()

	// Healthcheck
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	})

	// Ephemeral session endpoint
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if apiKey == "" {
			http.Error(w, "server not configured: OPENAI_API_KEY missing", http.StatusInternalServerError)
			return
		}

		payload := sessionRequest{
			Model:      model,
			Voice:      voice,
			Modalities: []string{"text", "audio"},
		}
		b, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/realtime/sessions", bytes.NewReader(b))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		// Some deployments require this beta header for Realtime
		req.Header.Set("OpenAI-Beta", "realtime=v1")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("create session failed: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})

	// Static files: serve the frontend directory.
	fs := http.FileServer(http.Dir(feDir))
	mux.Handle("/", fs)

	srv := &http.Server{
		Addr:              ":" + addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	abs, _ := filepath.Abs(feDir)
	log.Printf("Serving fe from %s at http://localhost:%s\n", abs, addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
