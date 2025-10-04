package middleware

import (
	"log"
	"net/http"
	"strconv" // Import for string conversion

	"gtglivemap/worker" // We need the Redis client (RDB)

	"github.com/gin-gonic/gin"
	limit "github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/redis"
)

func GinLimitMiddleware(limiter *limit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		context, err := limiter.Get(c.Request.Context(), c.ClientIP())
		if err != nil {
			log.Printf("ERROR: Could not get rate limit context: %v", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		c.Header("X-RateLimit-Limit", strconv.FormatInt(context.Limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(context.Remaining, 10))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(context.Reset, 10))

		if context.Reached {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}

		c.Next()
	}
}

// CreateRateLimiter creates a new rate limiter instance.
func CreateRateLimiter(formattedRate string) (*limit.Limiter, error) {
	rate, err := limit.NewRateFromFormatted(formattedRate)
	if err != nil {
		return nil, err
	}

	store, err := redis.NewStore(worker.RDB)
	if err != nil {
		return nil, err
	}

	instance := limit.New(store, rate)

	return instance, nil
}
