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
