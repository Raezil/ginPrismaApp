package router

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter holds the rate limiter configuration
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rps rate.Limit, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rps,
		burst:    burst,
	}
}

// GetLimiter returns the rate limiter for a given key (IP address)
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}

	return limiter
}

// CleanupExpiredLimiters removes expired limiters to prevent memory leaks
func (rl *RateLimiter) CleanupExpiredLimiters() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for key, limiter := range rl.limiters {
				// Remove limiters that haven't been used in the last 5 minutes
				if limiter.TokensAt(time.Now()) == float64(rl.burst) {
					delete(rl.limiters, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use client IP as the key
		key := c.ClientIP()
		limiter := rl.GetLimiter(key)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// StrictRateLimitMiddleware creates a stricter rate limiting middleware for sensitive endpoints
func StrictRateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		limiter := rl.GetLimiter(key)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": "60s",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
