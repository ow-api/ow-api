package cache

import (
	"github.com/bluele/gcache"
	"net/url"
	"strconv"
	"time"
)

type Gcache struct {
	cache gcache.Cache
}

func NewGcache(u *url.URL) *Gcache {
	size := 128

	q := u.Query()

	if sizeStr := q.Get("size"); sizeStr != "" {
		var err error
		size, err = strconv.Atoi(sizeStr)

		if err != nil {
			size = 128
		}
	}

	gc := gcache.New(size).
		LRU().
		Build()

	return &Gcache{cache: gc}
}

func (c *Gcache) Get(key string) ([]byte, error) {
	res, err := c.cache.Get(key)

	if err != nil {
		return nil, err
	}

	return res.([]byte), err
}

func (c *Gcache) Set(key string, b []byte, d time.Duration) error {
	return c.cache.SetWithExpire(key, b, d)
}
