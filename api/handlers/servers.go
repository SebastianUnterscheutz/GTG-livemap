// api/handlers/servers.go
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"gtglivemap/cache"
	"gtglivemap/database"
	"gtglivemap/models"
	"gtglivemap/pkg/moderation"
	"gtglivemap/utils"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var validServerNameRegex = regexp.MustCompile(`^[\w\s\[\]-]+$`)

func GetUserServersHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDUint64 := userID.(uint64)

	// This structure contains ALLE Felder für Owner/Admins
	type FullServerResponse struct {
		ID                uuid.UUID        `json:"id"`
		Name              string           `json:"name"`
		IsPublic          bool             `json:"is_public"`
		IsListed          bool             `json:"is_listed"`
		MapConfig         models.MapConfig `json:"map_config"`
		LogSourceType     string           `json:"LogSourceType"`
		FtpHost           string           `json:"FtpHost"`
		FtpPort           int              `json:"FtpPort"`
		FtpUser           string           `json:"FtpUser"`
		UseFtps           bool             `json:"UseFtps"`
		ProfileFolderPath string           `json:"ProfileFolderPath"`
		HasPasswordSet    bool             `json:"has_password_set"`
		HasSshKeySet      bool             `json:"has_ssh_key_set"`
	}

	// ★★★ NEUE STRUKTUR FÜR GETEILTE SERVER ★★★
	// This structure contains nur die absolut notwendigen Informationen.
	type SharedServerResponse struct {
		ID        uuid.UUID        `json:"id"`
		Name      string           `json:"name"`
		MapConfig models.MapConfig `json:"map_config"`
	}

	type GroupedServerResponse struct {
		Owned  []FullServerResponse   `json:"owned"`
		Shared []SharedServerResponse `json:"shared"` // Verwendet die neue, sichere Struktur
		Admin  []FullServerResponse   `json:"admin"`
	}

	response := GroupedServerResponse{
		Owned:  []FullServerResponse{},
		Shared: []SharedServerResponse{},
		Admin:  []FullServerResponse{},
	}

	// Helper function for volle Details (Owner/Admin)
	makeFullResponse := func(s models.Server) FullServerResponse {
		s.MapConfig.TilesURL = rewriteTilesURL(s.MapConfig.TilesURL)
		return FullServerResponse{
			ID:                s.ID,
			Name:              s.Name,
			IsPublic:          s.IsPublic,
			IsListed:          s.IsListed,
			MapConfig:         s.MapConfig,
			LogSourceType:     s.LogSourceType,
			FtpHost:           s.FtpHost,
			FtpPort:           s.FtpPort,
			FtpUser:           s.FtpUser,
			UseFtps:           s.UseFtps,
			ProfileFolderPath: s.ProfileFolderPath,
			HasPasswordSet:    len(s.EncryptedPassword) > 0,
			HasSshKeySet:      len(s.EncryptedSshKey) > 0,
		}
	}

	// ★★★ NEUE HELFERFUNKTION FÜR GETEILTE SERVER ★★★
	makeSharedResponse := func(s models.Server) SharedServerResponse {
		s.MapConfig.TilesURL = rewriteTilesURL(s.MapConfig.TilesURL)
		return SharedServerResponse{
			ID:        s.ID,
			Name:      s.Name,
			MapConfig: s.MapConfig,
		}
	}

	cacheKey := fmt.Sprintf("dashboard:servers:%d", userIDUint64)

	var cachedResponse GroupedServerResponse // Definiere die Zielstruktur hier

	// Versuche, die Daten aus dem Cache zu lesen.
	if err := cache.Get(cacheKey, &cachedResponse); err == nil {
		// Cache hit! Sende die gecachten Daten und beende die Funktion.
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	var user models.User
	database.DB.First(&user, userIDUint64)

	// 1. Eigene Server holen
	var ownedServers []models.Server
	database.DB.Preload("MapConfig").Where("owner_id = ?", userIDUint64).Order("name asc").Find(&ownedServers)
	for _, s := range ownedServers {
		response.Owned = append(response.Owned, makeFullResponse(s))
	}

	// 2. Geteilte Server holen und die neue Funktion verwenden
	var sharedAccesses []models.ServerAccess
	database.DB.Preload("Server.MapConfig").Where("user_id = ?", userIDUint64).Find(&sharedAccesses)
	for _, access := range sharedAccesses {
		isOwner := false
		for _, owned := range ownedServers {
			if owned.ID == access.ServerID {
				isOwner = true
				break
			}
		}
		if !isOwner {
			response.Shared = append(response.Shared, makeSharedResponse(access.Server))
		}
	}

	// 3. Admin-Server holen
	if user.AccountType == "admin" {
		var existingIDs []uuid.UUID
		for _, s := range response.Owned {
			existingIDs = append(existingIDs, s.ID)
		}
		// Hier Shared IDs aus dem Originalobjekt holen
		for _, a := range sharedAccesses {
			isOwner := false
			for _, o := range ownedServers {
				if o.ID == a.ServerID {
					isOwner = true
					break
				}
			}
			if !isOwner {
				existingIDs = append(existingIDs, a.ServerID)
			}
		}

		var adminServers []models.Server
		query := database.DB.Preload("MapConfig").Order("name asc")
		if len(existingIDs) > 0 {
			query = query.Where("id NOT IN ?", existingIDs)
		}
		query.Find(&adminServers)

		for _, s := range adminServers {
			response.Admin = append(response.Admin, makeFullResponse(s))
		}
	}

	cache.Set(cacheKey, response, 30*time.Second)

	c.JSON(http.StatusOK, response)
}

func CreateServerHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var user models.User
	if err := database.DB.First(&user, userID.(uint64)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Authenticated user not found."})
		return
	}

	// 2. Bestimme das anzuwendende Limit.
	maxServersLimit := user.MaxServers
	if maxServersLimit == 0 {
		maxServersLimit = 10
	}

	// 3. Zähle die aktuellen Server des Benutzers.
	var serverCount int64
	database.DB.Model(&models.Server{}).Where("owner_id = ?", userID.(uint64)).Count(&serverCount)

	// 4. Vergleiche mit dem dynamischen Limit.
	if serverCount >= int64(maxServersLimit) {
		log.Printf("User %d tried to create a new server but has reached their limit of %d.", userID.(uint64), maxServersLimit)
		c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("You have reached your server limit of %d.", maxServersLimit)})
		return
	}

	var req struct {
		Name        string   `json:"name" binding:"required"`
		MapConfigID FlexUint `json:"map_config_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// VALIDIERUNG
	if !validServerNameRegex.MatchString(req.Name) || len(req.Name) > 50 { // Add length limit
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server name contains invalid characters or is too long. Allowed: a-z, 0-9, [], -, _"})
		return
	}

	if !moderation.IsClean(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server name contains inappropriate words."})
		return
	}

	rawAPIKeyBytes := make([]byte, 32)
	rand.Read(rawAPIKeyBytes)
	rawAPIKey := hex.EncodeToString(rawAPIKeyBytes)

	hashedAPIKey, err := bcrypt.GenerateFromPassword([]byte(rawAPIKey), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate API key"})
		return
	}

	newServer := models.Server{
		OwnerID:     userID.(uint64),
		MapConfigID: uint(req.MapConfigID),
		Name:        req.Name,
		APIKey:      string(hashedAPIKey),
		IsPublic:    false,
	}

	if err := database.DB.Create(&newServer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save server to database"})
		return
	}

	cacheKey := fmt.Sprintf("dashboard:servers:%d", userID.(uint64))

	cache.Delete(cacheKey)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Server created successfully!",
		"server_id": newServer.ID,
		"api_key":   rawAPIKey,
	})
}

func UpdateServerHandler(c *gin.Context) {
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)
	userID, _ := c.Get("user_id")

	var req struct {
		Name              string   `json:"name" binding:"required"`
		IsPublic          bool     `json:"is_public"`
		IsListed          bool     `json:"is_listed"`
		MapConfigID       FlexUint `json:"map_config_id"`
		LogSourceType     string   `json:"log_source_type"`
		FtpHost           string   `json:"ftp_host"`
		FtpPort           int      `json:"ftp_port"`
		FtpUser           string   `json:"ftp_user"`
		Password          string   `json:"password"`
		SshKey            string   `json:"ssh_key"`
		UseFtps           bool     `json:"use_ftps"`
		ProfileFolderPath string   `json:"profile_folder_path"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// VALIDIERUNG
	if !validServerNameRegex.MatchString(req.Name) || len(req.Name) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server name contains invalid characters or is too long. Allowed: a-z, 0-9, [], -, _"})
		return
	}
	if !moderation.IsClean(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server name contains inappropriate words."})
		return
	}

	updates := map[string]interface{}{
		"name":                req.Name,
		"is_public":           req.IsPublic,
		"is_listed":           req.IsListed,
		"map_config_id":       uint(req.MapConfigID),
		"log_source_type":     req.LogSourceType,
		"ftp_host":            req.FtpHost,
		"ftp_port":            req.FtpPort,
		"ftp_user":            req.FtpUser,
		"use_ftps":            req.UseFtps,
		"profile_folder_path": req.ProfileFolderPath,
	}

	if !req.IsPublic {
		updates["is_listed"] = false
	}

	if req.Password != "" {
		if req.Password == "_RESET_" {
			updates["encrypted_password"] = nil
		} else {
			encryptedPass, err := utils.Encrypt([]byte(req.Password))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt password"})
				return
			}
			updates["encrypted_password"] = encryptedPass
		}
	}
	// Same logic for den SSH-Key
	if req.SshKey != "" {
		if req.SshKey == "_RESET_" {
			updates["encrypted_ssh_key"] = nil
		} else {
			encryptedKey, err := utils.Encrypt([]byte(req.SshKey))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt ssh key"})
				return
			}
			updates["encrypted_ssh_key"] = encryptedKey
		}
	}

	result := database.DB.Model(&models.Server{}).Where("id = ?", serverID).Updates(updates)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update server in database"})
		return
	}

	if result.RowsAffected == 0 {
		log.Printf("Update for server %s resulted in 0 rows affected (no change).", serverID)
	}

	dashboardCacheKey := fmt.Sprintf("dashboard:servers:%d", userID.(uint64))
	if err := cache.Delete(dashboardCacheKey); err != nil {
		log.Printf("WARN: Konnte Dashboard-Cache für User %d nicht löschen: %v", userID.(uint64), err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "server updated successfully"})
}

// DeleteServerHandler deletes a server und alle zugehörigen Daten.
func DeleteServerHandler(c *gin.Context) {
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	var serverToDelete models.Server
	if err := database.DB.First(&serverToDelete, "id = ?", serverID).Error; err != nil {
		// If the server does not exist, nothing can be deleted either.
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// We delete all dependent Daten in einer Transaktion for data security.
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("server_id = ?", serverID).Delete(&models.PlayerPosition{}).Error; err != nil {
			return err
		}

		if err := tx.Where("server_id = ?", serverID).Delete(&models.DamageEvent{}).Error; err != nil {
			return err
		}

		if err := tx.Where("server_id = ?", serverID).Delete(&models.ServerAccess{}).Error; err != nil {
			return err
		}

		if err := tx.Where("server_id = ?", serverID).Delete(&models.Faction{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&models.Server{}, serverID).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete server and associated data"})
		return
	}

	dashboardCacheKey := fmt.Sprintf("dashboard:servers:%d", serverToDelete.OwnerID)
	if err := cache.Delete(dashboardCacheKey); err != nil {
		log.Printf("WARN: Konnte Dashboard-Cache für User %d nicht löschen: %v", serverToDelete.OwnerID, err)
	}

	// 2. Lösche den Timeline-Cache des gelöschten Servers.
	timelineCacheKey := fmt.Sprintf(cache.RecentTimestampsCacheKey, serverIDStr)
	if err := cache.Delete(timelineCacheKey); err != nil {
		log.Printf("WARN: Konnte Timeline-Cache für Server %s nicht löschen: %v", serverIDStr, err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "server deleted successfully"})
}

func RegenerateAPIKeyHandler(c *gin.Context) {
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	rawAPIKeyBytes := make([]byte, 32)
	rand.Read(rawAPIKeyBytes)
	rawAPIKey := hex.EncodeToString(rawAPIKeyBytes)

	// Hashe den neuen Key
	hashedAPIKey, err := bcrypt.GenerateFromPassword([]byte(rawAPIKey), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate new API key"})
		return
	}

	result := database.DB.Model(&models.Server{}).Where("id = ?", serverID).Update("api_key", string(hashedAPIKey))
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update API key in database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API Key regenerated successfully. Your old key is now invalid.",
		"api_key": rawAPIKey,
	})
}
