package middleware

import (
	"net/http"

	"golang.org/x/time/rate"
)

// RateLimit returns a middleware that limits requests to rps per second
// with the given burst size. Excess requests get 429 Too Many Requests.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
