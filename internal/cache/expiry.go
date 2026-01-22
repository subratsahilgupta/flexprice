package cache

import "time"

const (
	ExpiryDefaultInMemory = 30 * time.Minute
	ExpiryDefaultRedis    = 5 * time.Minute
)
