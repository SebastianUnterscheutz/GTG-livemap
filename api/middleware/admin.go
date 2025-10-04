// api/middleware/admin.go
package middleware

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminOnlyMiddleware restricts access to routes requiring admin privileges.
func AdminOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID_iface, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var user models.User
		if err := database.DB.First(&user, userID_iface.(uint64)).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}

		if user.AccountType != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin privileges required"})
			return
		}

		c.Next()
	}
}
