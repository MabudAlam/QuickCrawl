package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

func CORSMiddlewareGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	}
}

func AuthMiddlewareGin(apiKeys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(apiKeys) == 0 {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			for _, key := range apiKeys {
				if key == "" {
					c.Next()
					return
				}
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, types.APIErr[struct{}]("Missing Authorization header"))
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, types.APIErr[struct{}]("Invalid authorization header"))
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		valid := false
		for _, key := range apiKeys {
			if constantTimeEq([]byte(key), []byte(token)) {
				valid = true
				break
			}
		}

		if !valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, types.APIErr[struct{}]("Invalid API key"))
			return
		}

		c.Next()
	}
}

type RateLimiter struct {
	requests map[string][]time.Time
	maxRate  int
	window   time.Duration
	mu       sync.Mutex
}

func NewRateLimiter(maxRate int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		maxRate:  maxRate,
		window:   window,
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	windowStart := now.Add(-rl.window)

	times := rl.requests[ip]
	valid := times[:0]
	for _, t := range times {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxRate {
		rl.requests[ip] = valid
		return false
	}

	rl.requests[ip] = append(valid, now)
	return true
}

func RateLimitMiddlewareGin(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := getClientIPGin(c)
		if !limiter.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, types.APIErr[struct{}]("Rate limited"))
			return
		}
		c.Next()
	}
}

func getClientIPGin(c *gin.Context) string {
	xff := c.GetHeader("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	xri := c.GetHeader("X-Real-IP")
	if xri != "" {
		return xri
	}
	return c.ClientIP()
}

func constantTimeEq(a, b []byte) bool {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	var result byte
	if len(a) != len(b) {
		result = 1
	}
	for i := 0; i < maxLen; i++ {
		var x, y byte
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		result |= x ^ y
	}
	return subtle.ConstantTimeByteEq(result, 0) == 1
}
