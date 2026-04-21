package main

import (
	"log"
	"net/http"
	"os"
	"server/handlers"
	"server/middleware"

	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not loaded, relying on system environment variables")
	}

	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Println("Warning: GEMINI_API_KEY environment variable not set. Chatbot functionality will not work properly.")
	}
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.Handle("/api/login", middleware.LoginRateLimit(http.HandlerFunc(handlers.LoginHandler)))
	mux.HandleFunc("/api/logout", handlers.LogoutHandler)
	// Protect sensitive routes with JWT middleware
	mux.Handle("/api/getAllQueries", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.GetAllQueries)))
	mux.HandleFunc("/api/add-query", handlers.AddQuery)
	mux.Handle("/api/updateStatus", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.UpdateQueryStatus)))
	mux.Handle("/api/analytics", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.AnalyticsHandler)))
	mux.HandleFunc("/api/chat", handlers.ChatHandler)
	mux.Handle("/api/check-login", handlers.JWTAuthMiddleware(http.HandlerFunc(handlers.CheckLoginHandler)))

	c := cors.New(cors.Options{
		AllowedOrigins: []string{
			"https://rakshak-service.vercel.app",
			"https://rakshak-service-ayushsingh-22s-projects.vercel.app",
			"https://rakshak-service-git-main-ayushsingh-22s-projects.vercel.app",
			"http://localhost:3000",
		},
		AllowCredentials: true,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
	})

	// Start server
	log.Println("Server is running on localhost.")
	if err := http.ListenAndServe(":8080", c.Handler(mux)); err != nil {
		log.Fatal(err)
	}
}
