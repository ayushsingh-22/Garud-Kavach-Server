package main

import (
	"log"
	"net/http"
	"os"
	"server/db"
	"server/handlers"
	"server/middleware"
	"server/services"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not loaded, relying on system environment variables")
	}

	db.Init()

	// Start background services
	services.StartEmailWorker()
	services.StartGuardLicenseChecker(db.DB)

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
	r.HandleFunc("/api/register", handlers.RegisterCustomerHandler).Methods("POST")
	r.Handle("/api/login", middleware.LoginRateLimit(http.HandlerFunc(handlers.LoginHandler))).Methods("POST")
	r.HandleFunc("/api/logout", handlers.LogoutHandler).Methods("POST")
	r.HandleFunc("/api/add-query", handlers.AddQuery).Methods("POST")
	r.Handle("/api/chat", middleware.ChatRateLimit(http.HandlerFunc(handlers.ChatHandler))).Methods("POST")

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

	// -- Guards Routes (SuperAdmin, Manager, HR)
	guardsRouter := s.PathPrefix("/guards").Subrouter()
	guardsRouter.Use(handlers.RequireRole("superadmin", "manager", "hr"))
	guardsRouter.HandleFunc("", handlers.GetGuards).Methods("GET")
	guardsRouter.HandleFunc("/{id:[0-9]+}", handlers.GetGuardByID).Methods("GET")
	guardsRouter.HandleFunc("", handlers.CreateGuard).Methods("POST")
	guardsRouter.HandleFunc("/expiring", handlers.GetExpiringGuards).Methods("GET")
	guardsRouter.HandleFunc("/{id:[0-9]+}", handlers.UpdateGuard).Methods("PUT")
	guardsRouter.HandleFunc("/{id:[0-9]+}", handlers.SoftDeleteGuard).Methods("DELETE")
	guardsRouter.HandleFunc("/{id:[0-9]+}/assign", handlers.AssignGuard).Methods("POST")

	// -- SuperAdmin Only Routes
	adminRouter := s.PathPrefix("/admin").Subrouter()
	adminRouter.Use(handlers.RequireRole("superadmin"))
	adminRouter.HandleFunc("/users", handlers.GetAdminUsers).Methods("GET")
	adminRouter.HandleFunc("/users", handlers.CreateAdminUser).Methods("POST")
	adminRouter.HandleFunc("/users/{id}", handlers.UpdateAdminUser).Methods("PUT")
	adminRouter.HandleFunc("/users/{id}", handlers.SoftDeleteUser).Methods("DELETE")

	// -- Finance Routes
	financeRouter := s.PathPrefix("/finance").Subrouter()
	financeRouter.Use(handlers.RequireRole("superadmin", "finance"))
	financeRouter.HandleFunc("/invoices", handlers.GetInvoices).Methods("GET")
	financeRouter.HandleFunc("/reports", handlers.GetFinanceReports).Methods("GET")
	financeRouter.HandleFunc("/expenses", handlers.CreateExpense).Methods("POST")
	financeRouter.HandleFunc("/expenses", handlers.GetExpenses).Methods("GET")
	financeRouter.HandleFunc("/invoices/{id:[0-9]+}", handlers.UpdateInvoiceStatus).Methods("PUT")

	// -- HR Routes
	hrRouter := s.PathPrefix("/hr").Subrouter()
	hrRouter.Use(handlers.RequireRole("superadmin", "hr"))
	hrRouter.HandleFunc("/shifts", handlers.GetShifts).Methods("GET")
	hrRouter.HandleFunc("/shifts", handlers.CreateShift).Methods("POST")
	hrRouter.HandleFunc("/shifts/{id:[0-9]+}", handlers.UpdateShift).Methods("PUT")
	hrRouter.HandleFunc("/payroll", handlers.GetPayroll).Methods("GET")
	hrRouter.HandleFunc("/payroll/finalize", handlers.FinalizePayroll).Methods("POST")
	hrRouter.HandleFunc("/leaves", handlers.GetLeaves).Methods("GET")
	hrRouter.HandleFunc("/leaves/{id:[0-9]+}", handlers.UpdateLeaveStatus).Methods("PUT")
	hrRouter.HandleFunc("/guards/expiring", handlers.GetExpiringGuards).Methods("GET")

	// Allow manager to access audit logs as per Phase 3.6
	managerRouter.HandleFunc("/admin/audit-logs", handlers.GetAuditLogs).Methods("GET")

	// -- Customer Routes
	customerRouter := s.PathPrefix("/customer").Subrouter()
	customerRouter.Use(handlers.RequireRole("customer"))
	customerRouter.HandleFunc("/profile", handlers.GetCustomerProfile).Methods("GET")
	customerRouter.HandleFunc("/profile", handlers.UpdateCustomerProfile).Methods("PUT")
	customerRouter.HandleFunc("/password", handlers.UpdateCustomerPassword).Methods("PUT")
	customerRouter.HandleFunc("/queries", handlers.GetCustomerQueries).Methods("GET")

	// -- Notification Routes (all authenticated users)
	s.HandleFunc("/notifications", handlers.GetNotifications).Methods("GET")
	s.HandleFunc("/notifications/read", handlers.MarkNotificationsRead).Methods("POST")
	s.HandleFunc("/notifications/test", handlers.SendTestNotification).Methods("POST")

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
