// api/middleware/ownership.go

package middleware

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ServerOwnerOrAdminMiddleware stellt sicher, dass der eingeloggte Benutzer entweder
// der Besitzer des Servers ist ODER ein Administrator.
func ServerOwnerOrAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID_iface, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
			return
		}
		userID := userID_iface.(uint64)

		// 1. Hole den Benutzer aus der DB, um seine Rolle zu prüfen
		var user models.User
		if err := database.DB.First(&user, userID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authenticated user not found in database"})
			return
		}

		serverIDStr := c.Param("id")
		serverID, err := uuid.Parse(serverIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid server ID format"})
			return
		}

		var server models.Server
		if err := database.DB.First(&server, serverID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}

		if user.AccountType == "admin" {
			log.Printf("Admin Access: User %d (Admin) accessing Server ID %s", user.ID, server.ID)
		} else if server.OwnerID == userID {
			log.Printf("Owner Access: User %d accessing own Server ID %s", user.ID, server.ID)
		} else {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to manage this server"})
			return
		}

		c.Set("server", server)
		c.Next()
	}
}
