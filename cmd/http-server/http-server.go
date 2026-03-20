package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtPublicKey ed25519.PublicKey

type runCommandRequest struct {
	Cmd []string `json:"cmd"`
}

type runCommandResponse struct {
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

type statusResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	BuildDate string    `json:"build_date"`
	GitCommit string    `json:"git_commit"`
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func jwtAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtPublicKey, nil
		})

		if err != nil {
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func runCommandHandler(w http.ResponseWriter, r *http.Request) {

	var req runCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(runCommandResponse{
			Status: "error",
			Error:  "Invalid JSON body: " + err.Error(),
		})
		return
	}

	if len(req.Cmd) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(runCommandResponse{
			Status: "error",
			Error:  "cmd is required",
		})
		return
	}

	cmd := exec.Command(req.Cmd[0], req.Cmd[1:]...)
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(runCommandResponse{
			Status:   "completed",
			Output:   string(output),
			Error:    err.Error(),
			ExitCode: exitCode,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(runCommandResponse{
		Status:   "success",
		Output:   string(output),
		ExitCode: exitCode,
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statusResponse{
		Status:    "running",
		Timestamp: time.Now(),
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
	})
}

func loadJWTPublicKey(keyPath string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("expected PUBLIC KEY, got %s", block.Type)
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	ed25519PublicKey, ok := publicKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 public key")
	}

	return ed25519PublicKey, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Load JWT public key
	jwtKeysDir := os.Getenv("JWT_KEYS_DIR")
	if jwtKeysDir == "" {
		jwtKeysDir = "./jwt-keys"
	}

	// Try to find the public key file
	publicKeyPath := ""
	entries, err := os.ReadDir(jwtKeysDir)
	if err != nil {
		log.Fatalf("Failed to read jwt-keys directory: %v", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "-public.pem") {
			publicKeyPath = jwtKeysDir + "/" + entry.Name()
			break
		}
	}

	if publicKeyPath == "" {
		log.Fatalf("No public key found in %s", jwtKeysDir)
	}

	jwtPublicKey, err = loadJWTPublicKey(publicKeyPath)
	if err != nil {
		log.Fatalf("Failed to load JWT public key from %s: %v", publicKeyPath, err)
	}
	log.Printf("Loaded JWT public key from %s", publicKeyPath)

	// Create API router
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("POST /api/v1/run-command", jwtAuthMiddleware(runCommandHandler))
	mux.HandleFunc("/api/v1/status", statusHandler)
	mux.HandleFunc("/status", statusHandler)

	// Serve static files from ./www
	fs := http.FileServer(http.Dir("./www"))

	// Combine API and static file serving
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			loggingMiddleware(mux).ServeHTTP(w, r)
		} else {
			loggingMiddleware(fs).ServeHTTP(w, r)
		}
	})

	fmt.Printf("Serving ./www on http://127.0.0.1:%s\n", port)
	fmt.Printf("API endpoints:\n")
	fmt.Printf("  POST /api/v1/run-command (JWT auth required)\n")
	fmt.Printf("  GET  /api/v1/status\n")
	fmt.Printf("  GET  /status\n")
	if err := http.ListenAndServe("127.0.0.1:"+port, handler); err != nil {
		log.Fatal(err)
	}
}
