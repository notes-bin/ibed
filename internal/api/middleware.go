package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "Missing token")
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(h.config.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			respondError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), "user_id", claims["user_id"].(string))
		ctx = context.WithValue(ctx, "username", claims["username"].(string))
		ctx = context.WithValue(ctx, "is_admin", claims["is_admin"].(bool))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin := r.Context().Value("is_admin").(bool)
		if !isAdmin {
			respondError(w, http.StatusForbidden, "Admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RateLimitMiddleware(limit int, duration int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(float64(limit)/float64(duration)), limit)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := limiter.Wait(r.Context()); err != nil {
				respondError(w, http.StatusTooManyRequests, "Rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
