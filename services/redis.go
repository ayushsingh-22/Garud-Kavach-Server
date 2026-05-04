package services

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient is nil when REDIS_URL is not set or the connection fails.
// All callers must guard with IsRedisAvailable() before using it.
var RedisClient *redis.Client

// InitRedis reads REDIS_URL and attempts to connect.
// If the variable is unset or the ping fails, RedisClient stays nil and
// the application continues with its in-memory fallbacks.
func InitRedis() {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Println("[redis] REDIS_URL not set — running with in-memory fallbacks")
		return
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		log.Printf("[redis] invalid REDIS_URL: %v — running with in-memory fallbacks", err)
		return
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		log.Printf("[redis] ping failed: %v — running with in-memory fallbacks", err)
		return
	}

	RedisClient = client
	log.Println("[redis] connected successfully")
}

// IsRedisAvailable returns true when an active Redis connection exists.
func IsRedisAvailable() bool {
	return RedisClient != nil
}

// RedisGet is a helper that returns ("", false) on any error or cache miss.
func RedisGet(ctx context.Context, key string) (string, bool) {
	if !IsRedisAvailable() {
		return "", false
	}
	val, err := RedisClient.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

// RedisSet stores a value with a TTL. Silently ignores errors.
func RedisSet(ctx context.Context, key string, value any, ttl time.Duration) {
	if !IsRedisAvailable() {
		return
	}
	_ = RedisClient.Set(ctx, key, value, ttl).Err()
}

// RedisDel deletes a key. Silently ignores errors.
func RedisDel(ctx context.Context, key string) {
	if !IsRedisAvailable() {
		return
	}
	_ = RedisClient.Del(ctx, key).Err()
}

// RedisIncrWithExpire atomically increments a counter and sets TTL on first increment.
// Returns the new count and any error.
func RedisIncrWithExpire(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if !IsRedisAvailable() {
		return 0, nil
	}
	pipe := RedisClient.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
