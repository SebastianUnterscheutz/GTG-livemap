package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIGetAccessListHandler listet die Discord-User-IDs auf, die Zugriff auf den Server haben.
func APIGetAccessListHandler(c *gin.Context) {
	serverID_iface, _ := c.Get("server_id")
	serverID := serverID_iface.(uuid.UUID)

	var userIDs []uint64
	database.DB.Model(&models.ServerAccess{}).Where("server_id = ?", serverID).Pluck("user_id", &userIDs)

	// Wir geben bewusst nur die IDs zurück, da wir für nicht existierende User keine Namen/Avatare haben.
	c.JSON(http.StatusOK, userIDs)
}

// APIGrantAccessHandler gewährt einem Benutzer Zugriff. Der Benutzer muss NICHT existieren.
func APIGrantAccessHandler(c *gin.Context) {
	serverID_iface, _ := c.Get("server_id")
	serverID := serverID_iface.(uuid.UUID)

	var req struct {
		UserID uint64 `json:"user_id,string" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: user_id (string) is required"})
		return
	}

	var server models.Server
	database.DB.First(&server, "id = ?", serverID)
	if req.UserID == server.OwnerID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The server owner always has access and cannot be managed."})
		return
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		targetUser := models.User{
			ID:       req.UserID,
			Username: strconv.FormatUint(req.UserID, 10),
		}

		if err := tx.FirstOrCreate(&targetUser).Error; err != nil {
			log.Printf("Error in transaction (FirstOrCreate User): %v", err)
			return err
		}

		newAccess := models.ServerAccess{
			UserID:   req.UserID,
			ServerID: serverID,
		}
		if err := tx.FirstOrCreate(&newAccess).Error; err != nil {
			log.Printf("Error in transaction (FirstOrCreate Access): %v", err)
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to grant access due to a database error."})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Access granted successfully. User created if they did not exist."})
}

// APIRevokeAccessHandler entzieht einem Benutzer den Zugriff.
func APIRevokeAccessHandler(c *gin.Context) {
	serverID_iface, _ := c.Get("server_id")
	serverID := serverID_iface.(uuid.UUID)

	userIDToRevokeStr := c.Param("user_id")
	userIDToRevoke, err := strconv.ParseUint(userIDToRevokeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user_id format in URL"})
		return
	}

	// Sicherheitshalber den Owner vor dem Löschen schützen.
	var server models.Server
	database.DB.First(&server, "id = ?", serverID)
	if userIDToRevoke == server.OwnerID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "The server owner's access cannot be revoked."})
		return
	}

	result := database.DB.Where("server_id = ? AND user_id = ?", serverID, userIDToRevoke).Delete(&models.ServerAccess{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error during revoke"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Access entry not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Access revoked successfully"})
}
