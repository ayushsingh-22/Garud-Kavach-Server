module server

go 1.25.0

replace server => ./...

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.12.3
	github.com/rs/cors v1.11.1
	golang.org/x/crypto v0.50.0
	golang.org/x/time v0.15.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudinary/cloudinary-go/v2 v2.15.0 // indirect
	github.com/creasty/defaults v1.7.0 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/schema v1.4.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/redis/go-redis/v9 v9.19.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)
