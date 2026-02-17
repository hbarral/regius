package regius

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/hbarral/regius/cache"
)

type RateLimiterAlgorithm string

const (
	RateLimiterAlgorithmTokenBucket   RateLimiterAlgorithm = "token_bucket"
	RateLimiterAlgorithmSlidingWindow RateLimiterAlgorithm = "sliding_window"
)

type RateLimiterConfig struct {
	Enabled    bool
	Algorithm  RateLimiterAlgorithm
	Requests   int
	Window     time.Duration
	Storage    string
	TrustProxy bool
	Whitelist  []string
}

type rateLimiter struct {
	config     RateLimiterConfig
	storage    rateLimiterStorage
	inMemory   map[string]interface{}
	inMemoryMu sync.RWMutex
	middleware func(http.Handler) http.Handler
}

type tokenBucket struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
	mu         sync.Mutex
}

type slidingWindow struct {
	timestamps []time.Time
	windowSize time.Duration
	maxCount   int
	mu         sync.Mutex
}

type rateLimiterStorage interface {
	Get(key string) (interface{}, error)
	Set(key string, value interface{}, expiration time.Duration) error
	Delete(key string) error
	Increment(key string, expiration time.Duration) (int64, error)
}

type redisRateLimiterStorage struct {
	conn *redis.Pool
}

func (r *redisRateLimiterStorage) Get(key string) (interface{}, error) {
	conn := r.conn.Get()
	defer conn.Close()

	val, err := conn.Do("GET", key)
	if err != nil {
		return nil, err
	}

	if val == nil {
		return nil, nil
	}

	return val, nil
}

func (r *redisRateLimiterStorage) Set(key string, value interface{}, expiration time.Duration) error {
	conn := r.conn.Get()
	defer conn.Close()

	_, err := conn.Do("SETEX", key, int(expiration.Seconds()), value)
	return err
}

func (r *redisRateLimiterStorage) Delete(key string) error {
	conn := r.conn.Get()
	defer conn.Close()

	_, err := conn.Do("DEL", key)
	return err
}

func (r *redisRateLimiterStorage) Increment(key string, expiration time.Duration) (int64, error) {
	conn := r.conn.Get()
	defer conn.Close()

	val, err := redis.Int64(conn.Do("INCR", key))
	if err != nil {
		return 0, err
	}

	if val == 1 {
		_, err = conn.Do("EXPIRE", key, int(expiration.Seconds()))
		if err != nil {
			return 0, err
		}
	}

	return val, nil
}

type cacheRateLimiterStorage struct {
	cache cache.Cache
}

func (c *cacheRateLimiterStorage) Get(key string) (interface{}, error) {
	val, err := c.cache.Get(key)
	if err != nil {
		return nil, err
	}

	if val == nil {
		return nil, nil
	}

	return val, nil
}

func (c *cacheRateLimiterStorage) Set(key string, value interface{}, expiration time.Duration) error {
	return c.cache.Set(key, value, int(expiration.Seconds()))
}

func (c *cacheRateLimiterStorage) Delete(key string) error {
	return c.cache.Forget(key)
}

func (c *cacheRateLimiterStorage) Increment(key string, expiration time.Duration) (int64, error) {
	val, err := c.cache.Get(key)
	var valInt int64

	if err == nil && val != nil {
		switch v := val.(type) {
		case int:
			valInt = int64(v)
		case int64:
			valInt = v
		case string:
			valInt, _ = strconv.ParseInt(v, 10, 64)
		}
	}

	valInt++

	err = c.cache.Set(key, valInt, int(expiration.Seconds()))
	if err != nil {
		return 0, err
	}

	return valInt, nil
}

func newTokenBucket(maxTokens int, refillRate time.Duration) *tokenBucket {
	return &tokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) consume() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	if elapsed >= tb.refillRate {
		tb.tokens = tb.maxTokens
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

func newSlidingWindow(maxCount int, windowSize time.Duration) *slidingWindow {
	return &slidingWindow{
		timestamps: make([]time.Time, 0),
		windowSize: windowSize,
		maxCount:   maxCount,
	}
}

func (sw *slidingWindow) allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.windowSize)

	// Find all valid timestamps (within window)
	validTimestamps := make([]time.Time, 0)
	for _, ts := range sw.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	// Update with only valid timestamps
	sw.timestamps = validTimestamps

	if len(sw.timestamps) < sw.maxCount {
		sw.timestamps = append(sw.timestamps, now)
		return true
	}

	return false
}

func (r *rateLimiter) isWhitelisted(ip string) bool {
	for _, whitelistedIP := range r.config.Whitelist {
		if ip == whitelistedIP {
			return true
		}
	}
	return false
}

func (r *rateLimiter) getClientIP(req *http.Request) string {
	if r.config.TrustProxy {
		forwarded := req.Header.Get("X-Forwarded-For")
		if forwarded != "" {
			if forwarded[0] == '[' {
				if idx := len(forwarded); idx > 0 {
					forwarded = forwarded[1 : idx-1]
				}
			}
			return forwarded
		}

		realIP := req.Header.Get("X-Real-IP")
		if realIP != "" {
			return realIP
		}
	}

	ip := req.RemoteAddr
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}

	return ip
}

func (r *rateLimiter) checkLimit(identifier string) (bool, error) {
	if r.config.Storage == "redis" || r.config.Storage == "badger" {
		return r.checkDistributedLimit(identifier)
	}
	return r.checkInMemoryLimit(identifier)
}

func (r *rateLimiter) checkInMemoryLimit(identifier string) (bool, error) {
	r.inMemoryMu.Lock()
	defer r.inMemoryMu.Unlock()

	switch r.config.Algorithm {
	case RateLimiterAlgorithmTokenBucket:
		tb, exists := r.inMemory[identifier].(*tokenBucket)
		if !exists {
			tb = newTokenBucket(r.config.Requests, r.config.Window)
			r.inMemory[identifier] = tb
		}
		return tb.consume(), nil

	case RateLimiterAlgorithmSlidingWindow:
		sw, exists := r.inMemory[identifier].(*slidingWindow)
		if !exists {
			sw = newSlidingWindow(r.config.Requests, r.config.Window)
			r.inMemory[identifier] = sw
			log.Printf("[RATE LIMIT] Created new limiter for: %s", identifier)
		}
		allowed := sw.allow()
		log.Printf("[RATE LIMIT] %s - allowed=%v, timestamps=%d, window=%v", identifier, allowed, len(sw.timestamps), sw.windowSize)
		return allowed, nil

	default:
		return true, nil
	}
}

func (r *rateLimiter) checkDistributedLimit(identifier string) (bool, error) {
	switch r.config.Algorithm {
	case RateLimiterAlgorithmTokenBucket:
		key := fmt.Sprintf("ratelimit:tokenbucket:%s", identifier)
		val, err := r.storage.Get(key)
		if err != nil {
			return false, err
		}

		if val == nil {
			err := r.storage.Set(key, r.config.Requests, r.config.Window)
			if err != nil {
				return false, err
			}
			return true, nil
		}

		var tokens int
		switch v := val.(type) {
		case int:
			tokens = v
		case int64:
			tokens = int(v)
		case string:
			tokens, _ = strconv.Atoi(v)
		case []byte:
			tokens, _ = strconv.Atoi(string(v))
		default:
			tokens = 0
		}

		if tokens > 0 {
			err := r.storage.Set(key, tokens-1, r.config.Window)
			if err != nil {
				return false, err
			}
			return true, nil
		}

		return false, nil

	case RateLimiterAlgorithmSlidingWindow:
		key := fmt.Sprintf("ratelimit:slidingwindow:%s", identifier)
		count, err := r.storage.Increment(key, r.config.Window)
		if err != nil {
			return false, err
		}

		return count <= int64(r.config.Requests), nil

	default:
		return true, nil
	}
}

func (r *rateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !r.config.Enabled {
			next.ServeHTTP(w, req)
			return
		}

		ip := r.getClientIP(req)

		if r.isWhitelisted(ip) {
			next.ServeHTTP(w, req)
			return
		}

		path := req.URL.Path
		identifier := fmt.Sprintf("%s:%s", ip, path)

		allowed, err := r.checkLimit(identifier)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(r.config.Requests))
			w.Header().Set("X-RateLimit-Window", r.config.Window.String())
			w.Header().Set("Retry-After", strconv.Itoa(int(r.config.Window.Seconds())))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(r.config.Requests))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(r.config.Requests-1))
		w.Header().Set("X-RateLimit-Window", r.config.Window.String())
		next.ServeHTTP(w, req)
	})
}

func (r *Regius) RateLimiter(config RateLimiterConfig) func(next http.Handler) http.Handler {
	limiter := &rateLimiter{
		config:     config,
		inMemory:   make(map[string]interface{}),
		middleware: nil,
	}

	if config.Storage == "redis" && myRedisCache != nil {
		limiter.storage = &redisRateLimiterStorage{conn: redisPool}
	} else if config.Storage == "badger" && myBadgerCache != nil {
		limiter.storage = &cacheRateLimiterStorage{cache: r.Cache}
	} else {
		limiter.storage = nil
	}

	return limiter.Middleware
}
