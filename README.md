# Garud Kavach - Backend Server

This is the robust, scalable backend server for **Garud Kavach**, an enterprise-grade security service management platform. Built with Go, PostgreSQL, and Redis, it handles complex role-based operations, real-time guard tracking, payroll calculations, and AI-powered customer support.

## 🚀 Key Features

- **Role-Based Access Control (RBAC)**: Secure routes and data isolation for SuperAdmins, HR, Finance, Managers, and Customers.
- **Real-Time Guard Tracking**: WebSocket-based live location updates and geofence-based auto-assignment for security guards.
- **Advanced HR & Finance Modules**: Comprehensive management for guard directories, shifts, leaves, payroll processing, expenses, and automated invoicing.
- **AI Chatbot Integration**: Context-aware chatbot powered by the Gemini API for client assistance and service bookings.
- **Robust Database & Caching**: PostgreSQL for relational data integrity (with structured migrations) and Redis for rate-limiting, session handling, and background task queues.
- **Media & Email Management**: Cloudinary integration for image storage and asynchronous email queues for notifications.

## 🧰 Technology Stack

- **Language:** Go 1.25.0
- **Database:** PostgreSQL (lib/pq)
- **Caching & Queues:** Redis (go-redis)
- **Authentication:** JWT (golang-jwt) & secure HTTP-only cookies
- **Real-Time:** WebSockets (gorilla/websocket)
- **AI Integration:** Google Gemini API
- **Media Storage:** Cloudinary

## 📁 Folder Structure

```
Garud-Kavach-Server/
├── cmd/                # Utility scripts, seeders, and migration runners
├── db/                 # Database connection and setup logic
├── handlers/           # HTTP controllers for all API routes (Auth, HR, Finance, Customer, etc.)
├── helpers/            # Reusable utilities (validation, geospatial logic, audit logging)
├── middleware/         # HTTP interceptors (Rate Limiting, CORS, Headers, Auth)
├── migrations/         # SQL schema migrations (001 to 011)
├── models/             # Go structs and database schemas
├── services/           # External service integrations (Email, Redis, Chatbot Engine, Cloudinary)
├── main.go             # Application entry point and router configuration
└── .env                # Environment variables (DB, Redis, API Keys)
```

## 🔌 API Modules

The API is structured around different operational domains:

- **Auth (`/api/auth/*`)**: Login, signup, session validation, logout.
- **Customer (`/api/customer/*`)**: Query submission, profile management, booking history.
- **HR (`/api/hr/*`)**: Guard directory, shift assignments, leave approvals, payroll generation.
- **Finance (`/api/finance/*`)**: Invoices, expenses, P&L aggregation.
- **Guards (`/api/guards/*`)**: Real-time tracking (`/ws/tracking`), geofencing, auto-assignment.
- **Analytics (`/api/analytics`)**: Dashboard metrics, revenue trends, Top KPIs.
- **Chatbot (`/api/chatbot`)**: LLM-driven query resolution.

## 🛠️ Getting Started

### Prerequisites

- Go 1.25+
- PostgreSQL
- Redis Server
- [Gemini API Key](https://ai.google.dev/gemini-api/docs/get-started)
- Cloudinary Account (for media uploads)

### Installation & Setup

1. **Clone the repository:**
   ```sh
   git clone https://github.com/ayushsingh-22/Garud-Kavach-Server.git
   cd Garud-Kavach-Server
   ```

2. **Install dependencies:**
   ```sh
   go mod tidy
   ```

3. **Database Configuration:**
   - Create a PostgreSQL database (e.g., `garud_kavach`).
   - Run the SQL scripts located in the `migrations/` folder in order to set up your schema.

4. **Environment Variables:**
   Create a `.env` file in the root directory and configure the following:
   ```env
   # Database
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=postgres
   DB_PASSWORD=yourpassword
   DB_NAME=garud_kavach
   
   # Redis
   REDIS_URL=redis://localhost:6379/0
   
   # Secrets & APIs
   JWT_SECRET=your_jwt_secret
   GEMINI_API_KEY=your_gemini_key
   
   # Cloudinary
   CLOUDINARY_URL=cloudinary://API_KEY:API_SECRET@CLOUD_NAME
   
   # Server
   PORT=8080
   FRONTEND_URL=http://localhost:5173
   ```

5. **Run the server:**
   ```sh
   go run main.go
   ```
   The backend will start running on `http://localhost:8080`.

## ⚙️ Customization & Seeding

- Use the scripts in the `cmd/` folder (e.g., `cmd/seed_admin/main.go`, `cmd/seed_dummy_data/main.go`) to quickly populate your database for development.
- Configure rate limits and chatbot chat limits within the `middleware/` directory.

## 📝 License

This project is licensed under the MIT License - see the LICENSE file for details.
