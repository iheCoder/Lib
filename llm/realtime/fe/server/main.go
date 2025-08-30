package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
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

	// Show proxy related env for troubleshooting
	httpsProxy := os.Getenv("HTTPS_PROXY")
	allProxy := os.Getenv("ALL_PROXY")
	noProxy := os.Getenv("NO_PROXY")
	if httpsProxy != "" || allProxy != "" || noProxy != "" {
		log.Printf("Proxy env: HTTPS_PROXY set=%t ALL_PROXY set=%t NO_PROXY=%q", httpsProxy != "", allProxy != "", noProxy)
	}

	mux := http.NewServeMux()

	// Healthcheck
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	})

	// Ephemeral session endpoint
	httpClient := newHTTPClient()
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
		log.Printf("[/session] creating realtime session: model=%s voice=%s", payload.Model, payload.Voice)

		req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/realtime/sessions", bytes.NewReader(b))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		// Some deployments require this beta header for Realtime
		req.Header.Set("OpenAI-Beta", "realtime=v1")

		resp, err := httpClient.Do(req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, fmt.Sprintf(`{"error":"create session failed","detail":%q}`, err.Error()))
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		snippet := string(body)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "…"
		}
		log.Printf("[/session] upstream status=%d body=%s", resp.StatusCode, snippet)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	})

	// Network diagnostics: DNS + TCP dial to api.openai.com:443
	mux.HandleFunc("/debug/ping", func(w http.ResponseWriter, r *http.Request) {
		type Result struct {
			Host       string   `json:"host"`
			ResolvedIP []string `json:"resolved_ip"`
			DialMS     int64    `json:"dial_ms"`
			Error      string   `json:"error,omitempty"`
		}
		host := r.URL.Query().Get("host")
		if host == "" {
			host = "api.openai.com:443"
		}
		h, port, _ := net.SplitHostPort(host)
		if h == "" {
			h = host
		}
		ips, err := net.LookupIP(h)
		res := Result{Host: host}
		for _, ip := range ips {
			res.ResolvedIP = append(res.ResolvedIP, ip.String())
		}
		start := time.Now()
		conn, err := net.DialTimeout("tcp", host, 5*time.Second)
		if err != nil {
			res.Error = err.Error()
		} else {
			_ = conn.Close()
			_ = port // unused
			res.DialMS = time.Since(start).Milliseconds()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	})

	// Network diagnostics: simple OpenAI Models API call
	mux.HandleFunc("/debug/openai", func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" {
			http.Error(w, "OPENAI_API_KEY missing", http.StatusBadRequest)
			return
		}
		req, _ := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		// Optional beta header is harmless here
		req.Header.Set("OpenAI-Beta", "realtime=v1")
		t0 := time.Now()
		resp, err := httpClient.Do(req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, fmt.Sprintf(`{"error":"models request failed","detail":%q}`, err.Error()))
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		snippet := string(body)
		if len(snippet) > 800 {
			snippet = snippet[:800] + "…"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":%d,"took_ms":%d,"body":%q}`, resp.StatusCode, time.Since(t0).Milliseconds(), snippet)
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
	log.Printf("Serving fe from %s at http://localhost:%s", abs, addr)
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

func newHTTPClient() *http.Client {
	// Honor system proxies via environment. Add sane timeouts.
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: os.Getenv("INSECURE_SKIP_VERIFY") == "1",
		},
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Timeout: 60 * time.Second, Transport: tr}
}
