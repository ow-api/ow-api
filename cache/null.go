package cache

import "time"

type NullCache struct {
}

func (n *NullCache) Get(key string) ([]byte, error) {
	return nil, nil
}

func (n *NullCache) Set(key string, b []byte, d time.Duration) error {
	return nil
}
