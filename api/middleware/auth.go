package middleware

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

var (
	oauthConf   *oauth2.Config
	CookieStore *sessions.CookieStore // WAR `cookieStore`
)

// apiKeyCacheEntry stores all needed IDs for a server.
type apiKeyCacheEntry struct {
	serverID    uuid.UUID
	mapConfigID uint
	createdAt   time.Time
}

var (
	apiKeyCache = make(map[string]apiKeyCacheEntry)
	apiKeyMutex sync.RWMutex
	cacheTTL    = 5 * time.Minute
)

// APIKeyAuthMiddleware uses an in-memory cache to minimize expensive bcrypt checks.
func APIKeyAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		incomingAPIKey := c.GetHeader("X-API-KEY")
		if incomingAPIKey == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				incomingAPIKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if incomingAPIKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key or Bearer token is missing"})
			return
		}

		apiKeyMutex.RLock()
		entry, found := apiKeyCache[incomingAPIKey]
		apiKeyMutex.RUnlock()

		if found && time.Since(entry.createdAt) < cacheTTL {
			c.Set("server_id", entry.serverID)
			c.Set("map_config_id", entry.mapConfigID)
			c.Next()
			return
		}

		var servers []models.Server
		if err := database.DB.Select("id", "api_key", "map_config_id").Find(&servers).Error; err != nil {
			log.Printf("API Key Auth: Database error on fetching servers: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		var matchedServer *models.Server
		for _, server := range servers {
			err := bcrypt.CompareHashAndPassword([]byte(server.APIKey), []byte(incomingAPIKey))
			if err == nil {
				s := server
				matchedServer = &s
				break
			}
		}

		if matchedServer == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Invalid API Key"})
			return
		}

		apiKeyMutex.Lock()
		apiKeyCache[incomingAPIKey] = apiKeyCacheEntry{
			serverID:    matchedServer.ID,
			mapConfigID: matchedServer.MapConfigID,
			createdAt:   time.Now(),
		}
		apiKeyMutex.Unlock()

		log.Printf("API Key for server %s (map %d) validated and cached.", matchedServer.ID, matchedServer.MapConfigID)
		c.Set("server_id", matchedServer.ID)
		c.Set("map_config_id", matchedServer.MapConfigID)
		c.Next()
	}
}
