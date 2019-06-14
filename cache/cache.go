package cache

import (
	"net/url"
	"time"
)

type Provider interface {
	Get(key string) ([]byte, error)
	Set(key string, b []byte, d time.Duration) error
}

func ForURI(uri string) Provider {
	u, err := url.Parse(uri)

	if err != nil {
		return nil
	}

	if u.Scheme == "" && u.Path != "" {
		u.Scheme = u.Path
	}

	switch u.Scheme {
	case "memcached":
		return NewMemcached(u)
	case "redis":
		return NewRedisCache(u)
	case "gcache":
		return NewGcache(u)
	}

	return &NullCache{}
}
