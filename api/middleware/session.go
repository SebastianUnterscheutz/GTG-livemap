// api/middleware/session.go
package middleware

import (
	"gtglivemap/api/handlers" // Wir brauchen Zugriff auf den CookieStore aus auth.go
	"net/http"

	"github.com/gin-gonic/gin"
)

func SessionAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := handlers.CookieStore.Get(c.Request, "gtg-livemap-session")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid session state"})
			return
		}

		userID_iface := session.Values["user_id"]
		if userID_iface == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not logged in"})
			return
		}

		userID, ok := userID_iface.(uint64)
		if !ok || userID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user id in session"})
			return
		}

		c.Set("user_id", userID)

		c.Next()
	}
}
