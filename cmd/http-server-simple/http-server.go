package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fs := http.FileServer(http.Dir("./www"))
	handler := loggingMiddleware(fs)

	fmt.Printf("Serving ./www on http://127.0.0.1:%s\n", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, handler); err != nil {
		log.Fatal(err)
	}
}
