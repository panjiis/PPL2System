package middleware

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/middleware/stdlib"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

func RateLimit() gin.HandlerFunc {
	requestLimit := os.Getenv("RATE_LIMIT_REQUESTS")
	timeWindow := os.Getenv("RATE_LIMIT_TIME_WINDOW")

	log.Printf("Loading rate limit config - RATE_LIMIT_REQUESTS: '%s', RATE_LIMIT_TIME_WINDOW: '%s'",
		requestLimit, timeWindow)

	if requestLimit == "" {
		requestLimit = "100"
		log.Println("RATE_LIMIT_REQUESTS not set, using default: 100")
	}
	if timeWindow == "" {
		timeWindow = "minute"
		log.Println("RATE_LIMIT_TIME_WINDOW not set, using default: minute")
	}

	timeWindow = strings.TrimSpace(timeWindow)
	timeWindow = strings.TrimSuffix(strings.ToLower(timeWindow), "s")

	if len(timeWindow) == 0 {
		log.Println("Invalid RATE_LIMIT_TIME_WINDOW (empty after normalization), using default: minute")
		timeWindow = "minute"
	}

	validWindows := map[string]bool{
		"second": true,
		"minute": true,
		"hour":   true,
		"day":    true,
	}
	if !validWindows[timeWindow] {
		log.Printf("Invalid RATE_LIMIT_TIME_WINDOW '%s', using default: minute", timeWindow)
		timeWindow = "minute"
	}

	rateLimit := requestLimit + "-" + strings.ToUpper(string(timeWindow[0]))
	log.Printf("Rate limit configured: %s (%s requests per %s)", rateLimit, requestLimit, timeWindow)

	rate, err := limiter.NewRateFromFormatted(rateLimit)
	if err != nil {
		log.Fatalf("Error creating rate limiter: %v (rate format: %s)", err, rateLimit)
	}

	store := memory.NewStore()
	instance := limiter.New(store, rate)

	limiterMiddleware := stdlib.NewMiddleware(instance)

	return func(c *gin.Context) {
		limiterMiddleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)

		if c.Writer.Status() == http.StatusTooManyRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}
	}
}
