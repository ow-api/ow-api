package cache

import (
	"github.com/go-redis/redis"
	"net/url"
	"time"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(uri *url.URL) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:     uri.Host,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	return &RedisCache{client: client}
}

func (c *RedisCache) Get(key string) ([]byte, error) {
	return c.client.Get(key).Bytes()
}

func (c *RedisCache) Set(key string, b []byte, d time.Duration) error {
	return c.client.Set(key, b, d).Err()
}
