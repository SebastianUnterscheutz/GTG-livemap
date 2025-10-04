// api/handlers/users.go
package handlers

import (
	"fmt"
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func MeHandler(c *gin.Context) {
	userID, _ := c.Get("user_id") // Garantiert von der Middleware gesetzt

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found in database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"avatar":       toProxyAvatarURL(user.Avatar),
		"account_type": user.AccountType,
	})
}

// SearchUsersHandler searches for Benutzern anhand eines Namens-Präfixes.
func SearchUsersHandler(c *gin.Context) {
	// Only logged-in Benutzer dürfen andere Benutzer suchen.
	searchQuery := c.Query("username")
	if len(searchQuery) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query must be at least 3 characters long"})
		return
	}

	var users []models.User
	// Suche nach Benutzern, deren Name mit dem Query beginnt (LIKE 'name%')
	database.DB.Where("username LIKE ?", searchQuery+"%").Limit(10).Find(&users)

	// Send only sichere Daten zurück
	type UserSearchResponse struct {
		ID       uint64 `json:"id,string"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	}
	response := make([]UserSearchResponse, len(users))
	for i, u := range users {
		response[i] = UserSearchResponse{ID: u.ID, Username: u.Username, Avatar: toProxyAvatarURL(u.Avatar)}
	}
	c.JSON(http.StatusOK, response)
}

var discordAvatarURLRegex = regexp.MustCompile(`.*/avatars/(\d+)/([a-zA-Z0-9_]+)\.png.*`)

// toProxyAvatarURL konvertiert eine volle Discord-URL in unsere lokale Proxy-URL.
// If the URL doesn't match, gibt sie einen leeren String zurück.
func toProxyAvatarURL(discordURL string) string {
	matches := discordAvatarURLRegex.FindStringSubmatch(discordURL)
	if len(matches) == 3 {
		userID := matches[1]
		avatarHash := matches[2]
		return fmt.Sprintf("/api/v1/public/avatars/%s/%s", userID, avatarHash)
	}
	return "" // Fallback
}

func AdminSetServerLimitHandler(c *gin.Context) {
	// Benutzer-ID aus der URL holen
	targetUserIDStr := c.Param("user_id")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format in URL"})
		return
	}

	// Request-Body auslesen
	var req struct {
		MaxServers uint `json:"max_servers"` // Benutze uint, da ein Limit nicht negativ sein kann
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: 'max_servers' (number) is required."})
		return
	}

	// Update des Benutzers in der Datenbank
	result := database.DB.Model(&models.User{}).Where("id = ?", targetUserID).Update("max_servers", req.MaxServers)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user limit."})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found."})
		return
	}

	log.Printf("Admin %d set server limit for user %d to %d.", c.MustGet("user_id"), targetUserID, req.MaxServers)
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Successfully set server limit for user %d to %d.", targetUserID, req.MaxServers)})
}

// AdminGetAllUsersHandler fetches all Benutzer mit Zusatzinformationen für das Admin-Dashboard.
func AdminGetAllUsersHandler(c *gin.Context) {
	var users []models.User
	database.DB.Find(&users)

	// Eine Map, um die Anzahl der Server pro Benutzer zu speichern
	serverCounts := make(map[uint64]int64)
	type countResult struct {
		OwnerID uint64
		Count   int64
	}
	var results []countResult
	database.DB.Model(&models.Server{}).Select("owner_id, count(*) as count").Group("owner_id").Find(&results)
	for _, result := range results {
		serverCounts[result.OwnerID] = result.Count
	}

	// The response structure that alle needed Daten enthält
	type AdminUserResponse struct {
		ID               uint64 `json:"id,string"`
		Username         string `json:"username"`
		Avatar           string `json:"avatar"`
		AccountType      string `json:"account_type"`
		MaxServers       uint   `json:"max_servers"`
		OwnedServerCount int64  `json:"owned_server_count"`
	}

	response := make([]AdminUserResponse, len(users))
	for i, u := range users {
		response[i] = AdminUserResponse{
			ID:               u.ID,
			Username:         u.Username,
			Avatar:           toProxyAvatarURL(u.Avatar), // Wiederverwendung der Proxy-Funktion
			AccountType:      u.AccountType,
			MaxServers:       u.MaxServers,
			OwnedServerCount: serverCounts[u.ID], // Get the counted Anzahl
		}
	}

	c.JSON(http.StatusOK, response)
}

// AdminGetUserServersHandler fetches all Server (eigene und geteilte) eines bestimmten Benutzers.
func AdminGetUserServersHandler(c *gin.Context) {
	targetUserIDStr := c.Param("user_id")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format in URL"})
		return
	}

	// This structureen sind aus GetUserServersHandler bekannt
	type ServerResponse struct {
		ID        uuid.UUID        `json:"id"`
		Name      string           `json:"name"`
		MapConfig models.MapConfig `json:"map_config"`
	}
	type GroupedResponse struct {
		Owned  []ServerResponse `json:"owned"`
		Shared []ServerResponse `json:"shared"`
	}

	response := GroupedResponse{
		Owned:  []ServerResponse{},
		Shared: []ServerResponse{},
	}

	// 1. Eigene Server des Ziels holen
	var ownedServers []models.Server
	database.DB.Preload("MapConfig").Where("owner_id = ?", targetUserID).Order("name asc").Find(&ownedServers)
	for _, s := range ownedServers {
		response.Owned = append(response.Owned, ServerResponse{ID: s.ID, Name: s.Name, MapConfig: s.MapConfig})
	}

	// 2. Geteilte Server des Ziels holen
	var sharedAccesses []models.ServerAccess
	database.DB.Preload("Server.MapConfig").Where("user_id = ?", targetUserID).Find(&sharedAccesses)
	for _, access := range sharedAccesses {
		isOwner := false // Check if der Server nicht schon in der "owned"-Liste ist
		for _, owned := range ownedServers {
			if owned.ID == access.ServerID {
				isOwner = true
				break
			}
		}
		if !isOwner {
			response.Shared = append(response.Shared, ServerResponse{ID: access.Server.ID, Name: access.Server.Name, MapConfig: access.Server.MapConfig})
		}
	}

	c.JSON(http.StatusOK, response)
}
