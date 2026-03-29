package main

import (
	"log"
	"net/http"
	"os"
	"server/handlers"

	"github.com/rs/cors"
)

func main() {

	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Println("Warning: GEMINI_API_KEY environment variable not set. Chatbot functionality will not work properly.")
	}
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/login", handlers.LoginHandler)
	// Protect sensitive routes with JWT middleware
	mux.Handle("/api/getAllQueries", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.GetAllQueries)))
	// main.go
	mux.HandleFunc("/api/add-query", handlers.AddQuery)
	mux.Handle("/api/updateStatus", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.UpdateQueryStatus)))
	mux.Handle("/api/analytics", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.AnalyticsHandler)))
	mux.HandleFunc("/api/chat", handlers.ChatHandler)
	mux.Handle("/api/check-login", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.CheckLoginHandler)))

	c := cors.New(cors.Options{
		AllowedOrigins: []string{
			"https://rakshak-service.vercel.app",
			"https://rakshak-service-ayushsingh-22s-projects.vercel.app/",
			"https://rakshak-service-git-main-ayushsingh-22s-projects.vercel.app/",
			"http://localhost:3000", // for local development
			"http://localhost:8080", // if needed
		},
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
	})

	// Start server
	log.Println("Server is running on localhost.")
	if err := http.ListenAndServe(":8080", c.Handler(mux)); err != nil {
		log.Fatal(err)
	}
}
