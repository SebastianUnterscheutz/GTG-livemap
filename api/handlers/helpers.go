package handlers

import (
	"encoding/json"
	"errors"
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FlexUint uint

// UnmarshalJSON implements the logic to parse flexibly.
func (fu *FlexUint) UnmarshalJSON(data []byte) error {
	// First try to parse it as a normal number (e.g. 123).
	var num uint
	if err := json.Unmarshal(data, &num); err == nil {
		*fu = FlexUint(num)
		return nil
	}

	// If that fails, it might be a string (e.g. "123").
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err // Neither a valid number nor a valid string.
	}

	// It was a string, so we convert it to a number.
	parsed, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return err // The string does not contain a valid number.
	}

	*fu = FlexUint(parsed)
	return nil
}

// CheckServerAccess verifies if the current user has permission to access the specified server.
func CheckServerAccess(c *gin.Context, serverID uuid.UUID) (gin.H, int) {

	var server models.Server
	if err := database.DB.First(&server, "id = ?", serverID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gin.H{"error": "Server not found"}, http.StatusNotFound
		}
		return gin.H{"error": "Database error while fetching server"}, http.StatusInternalServerError
	}

	if server.IsPublic {
		return nil, 0
	}

	if server.IsListed {
		return nil, 0
	}

	session, err := CookieStore.Get(c.Request, "gtg-livemap-session")
	if err != nil {
		return gin.H{"error": "Access denied. You must be logged in to view this private server. (No Session)"}, http.StatusForbidden
	}

	userID_iface := session.Values["user_id"]
	if userID_iface == nil || userID_iface == "" {
		return gin.H{"error": "Access denied. You must be logged in to view this private server."}, http.StatusForbidden
	}
	userID := userID_iface.(uint64)

	if server.OwnerID == userID {
		return nil, 0
	}

	var user models.User
	database.DB.First(&user, userID)
	if user.AccountType == "admin" {
		return nil, 0
	}

	var access models.ServerAccess
	err = database.DB.Where("server_id = ? AND user_id = ?", serverID, userID).First(&access).Error
	if err == nil {
		return nil, 0
	}

	// If we reach this point, the user has no authorization.
	return gin.H{"error": "Access denied. You do not have permission to view this server."}, http.StatusForbidden
}

const proxyTilesPath = "/api/v1/tiles"

// rewriteTilesURL parses the originalURL and constructs the new proxy URL.
// Works for ANY https URL.
func rewriteTilesURL(originalURL string) string {
	if originalURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		log.Printf("Warning: Could not parse TilesURL '%s': %v", originalURL, err)
		return originalURL // In case of error, return the original URL
	}

	newURL := proxyTilesPath + "/" + parsedURL.Host + parsedURL.Path
	return newURL
}
