package cache

import (
	"github.com/bradfitz/gomemcache/memcache"
	"net/url"
	"strings"
	"time"
)

type Memcached struct {
	client *memcache.Client
}

func NewMemcached(u *url.URL) *Memcached {
	mc := memcache.New(strings.Split(u.Host, ",")...)

	return &Memcached{client: mc}
}

func (m *Memcached) Get(key string) ([]byte, error) {
	item, err := m.client.Get(key)

	if err != nil {
		return nil, err
	}

	return item.Value, nil
}

func (m *Memcached) Set(key string, b []byte, d time.Duration) error {
	return m.client.Set(&memcache.Item{Key: key, Value: b, Expiration: int32(d.Seconds())})
}
