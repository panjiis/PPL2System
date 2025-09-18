package middleware

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/middleware/stdlib"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

func RateLimit() gin.HandlerFunc {
	rate, err := limiter.NewRateFromFormatted("10-M")
	if err != nil {
		log.Fatalf("Error while running ratelimiter middleware")
	}

	store := memory.NewStore()
	instance := limiter.New(store, rate)

	limiterMiddleware := stdlib.NewMiddleware(instance)

	return func(c *gin.Context) {
		limiterMiddleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)

		if c.Writer.Status() == http.StatusTooManyRequests {
			c.Abort()
			return
		}
	}
}
