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

// apiKeyCacheEntry stores all needed IDs für einen Server.
type apiKeyCacheEntry struct {
	serverID    uuid.UUID
	mapConfigID uint // ★★★ ADDED ★★★
	createdAt   time.Time
}

var (
	// apiKeyCache speichert [roherAPIKey] -> cacheEintrag
	apiKeyCache = make(map[string]apiKeyCacheEntry)
	// apiKeyMutex protects den Cache vor gleichzeitigem Lesen/Schreiben.
	apiKeyMutex sync.RWMutex
	// cacheTTL indicates, how long a key is valid in the cache (z.B. 5 Minuten).
	cacheTTL = 5 * time.Minute
)

// APIKeyAuthMiddleware uses a In-Memory-Cache, um teure bcrypt checks.
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
			// ★ CACHE-TREFFER! ★
			c.Set("server_id", entry.serverID)
			c.Set("map_config_id", entry.mapConfigID) // ★★★ ADDED ★★★
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
			mapConfigID: matchedServer.MapConfigID, // ★★★ ADDED ★★★
			createdAt:   time.Now(),
		}
		apiKeyMutex.Unlock()

		log.Printf("API Key for server %s (map %d) validated and cached.", matchedServer.ID, matchedServer.MapConfigID)

		// Set both values in the context für den current request.
		c.Set("server_id", matchedServer.ID)
		c.Set("map_config_id", matchedServer.MapConfigID)
		c.Next()
	}
}
