package main

import (
	"log"
	"net/http"
	"os"
	"server/db"
	"server/handlers"
	"server/middleware"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not loaded, relying on system environment variables")
	}

	db.Init()

	// Set JWT key after loading environment
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	handlers.SetJwtKey([]byte(jwtSecret))

	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Println("Warning: GEMINI_API_KEY environment variable not set. Chatbot functionality will not work properly.")
	}
	r := mux.NewRouter()

	// Public Routes
	r.HandleFunc("/api/signup", handlers.SignUpHandler).Methods("POST")
	r.Handle("/api/login", middleware.LoginRateLimit(http.HandlerFunc(handlers.LoginHandler))).Methods("POST")
	r.HandleFunc("/api/logout", handlers.LogoutHandler).Methods("POST")
	r.HandleFunc("/api/add-query", handlers.AddQuery).Methods("POST")
	r.HandleFunc("/api/chat", handlers.ChatHandler).Methods("POST")

	// Authenticated Routes
	s := r.PathPrefix("/api").Subrouter()
	s.Use(handlers.JWTAuthMiddleware)

	// -- General Authenticated
	s.HandleFunc("/check-login", handlers.CheckLoginHandler).Methods("GET")

	// -- Manager & SuperAdmin Routes
	managerRouter := s.PathPrefix("").Subrouter()
	managerRouter.Use(handlers.RequireRole("superadmin", "manager"))
	managerRouter.HandleFunc("/updateStatus", handlers.UpdateQueryStatus).Methods("POST")
	managerRouter.HandleFunc("/getAllQueries", handlers.GetAllQueries).Methods("GET")
	managerRouter.HandleFunc("/analytics", handlers.AnalyticsHandler).Methods("GET")
	managerRouter.HandleFunc("/guards", handlers.GetGuards).Methods("GET")
	managerRouter.HandleFunc("/guards/{id:[0-9]+}", handlers.GetGuardByID).Methods("GET")
	managerRouter.HandleFunc("/guards", handlers.CreateGuard).Methods("POST")
	managerRouter.HandleFunc("/guards/expiring", handlers.GetExpiringGuards).Methods("GET")
	managerRouter.HandleFunc("/guards/{id:[0-9]+}", handlers.UpdateGuard).Methods("PUT")
	managerRouter.HandleFunc("/guards/{id:[0-9]+}", handlers.SoftDeleteGuard).Methods("DELETE")
	managerRouter.HandleFunc("/guards/{id:[0-9]+}/assign", handlers.AssignGuard).Methods("POST")

	// -- SuperAdmin Only Routes
	adminRouter := s.PathPrefix("/admin").Subrouter()
	adminRouter.Use(handlers.RequireRole("superadmin"))
	adminRouter.HandleFunc("/users", handlers.GetAdminUsers).Methods("GET")
	adminRouter.HandleFunc("/users", handlers.CreateAdminUser).Methods("POST")
	adminRouter.HandleFunc("/users/{id}", handlers.UpdateAdminUser).Methods("PUT")
	adminRouter.HandleFunc("/users/{id}", handlers.SoftDeleteUser).Methods("DELETE")

	// Allow manager to access audit logs as per Phase 3.6
	managerRouter.HandleFunc("/admin/audit-logs", handlers.GetAuditLogs).Methods("GET")

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
	log.Println("Server is running on localhost:8080.")
	if err := http.ListenAndServe(":8080", c.Handler(r)); err != nil {
		log.Fatal(err)
	}
}
