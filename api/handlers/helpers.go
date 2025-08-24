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

// UnmarshalJSON implementiert die Logik, um flexibel zu parsen.
func (fu *FlexUint) UnmarshalJSON(data []byte) error {
	// Zuerst versuchen, es als normale Zahl (z.B. 123) zu parsen.
	var num uint
	if err := json.Unmarshal(data, &num); err == nil {
		*fu = FlexUint(num)
		return nil
	}

	// Wenn das fehlschlägt, könnte es ein String (z.B. "123") sein.
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err // Weder eine gültige Zahl noch ein gültiger String.
	}

	// Es war ein String, also wandeln wir ihn in eine Zahl um.
	parsed, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return err // Der String enthält keine gültige Zahl.
	}

	*fu = FlexUint(parsed)
	return nil
}

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

	// Wenn wir hier ankommen, hat der Benutzer keine Berechtigung.
	return gin.H{"error": "Access denied. You do not have permission to view this server."}, http.StatusForbidden
}

const proxyTilesPath = "/api/v1/tiles"

// rewriteTilesURL parst die originalURL und baut die neue Proxy-URL zusammen.
// Funktioniert für JEDE https-URL.
func rewriteTilesURL(originalURL string) string {
	if originalURL == "" {
		return ""
	}

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		log.Printf("Warning: Could not parse TilesURL '%s': %v", originalURL, err)
		return originalURL // Im Fehlerfall die originale URL zurückgeben
	}

	newURL := proxyTilesPath + "/" + parsedURL.Host + parsedURL.Path
	return newURL
}
