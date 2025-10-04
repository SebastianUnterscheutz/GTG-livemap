// api/handlers/access.go
package handlers

import (
	"errors"
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetAccessListHandler returns all users who have access to a server.
func GetAccessListHandler(c *gin.Context) {
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	var accessList []models.ServerAccess
	database.DB.Preload("User").Where("server_id = ?", serverID).Find(&accessList)

	type AccessUserResponse struct {
		ID       uint64 `json:"id,string"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	}

	response := make([]AccessUserResponse, len(accessList))
	for i, access := range accessList {
		response[i] = AccessUserResponse{
			ID:       access.User.ID,
			Username: access.User.Username,
			Avatar:   toProxyAvatarURL(access.User.Avatar),
		}
	}
	c.JSON(http.StatusOK, response)
}

// GrantAccessHandler grants a user access to a server.
func GrantAccessHandler(c *gin.Context) {
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	var req struct {
		UserID uint64 `json:"user_id,string" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request, user_id is required"})
		return
	}

	loggedInUserID, _ := c.Get("user_id")
	server, _ := c.Get("server")
	serverModel := server.(models.Server)

	if req.UserID == loggedInUserID.(uint64) || req.UserID == serverModel.OwnerID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot grant access to yourself or the owner."})
		return
	}

	var targetUser models.User
	err := database.DB.First(&targetUser, req.UserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "User to grant access to not found. They must log in to the Live Map at least once."})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error while checking user."})
		return
	}
	// ★★★ End of new check ★★★

	newAccess := models.ServerAccess{
		UserID:   req.UserID,
		ServerID: serverID,
	}

	if err := database.DB.FirstOrCreate(&newAccess).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to grant access"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "access granted successfully"})
}

// RevokeAccessHandler revokes a user's access to a server.
func RevokeAccessHandler(c *gin.Context) {
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	userIDToRevokeStr := c.Param("user_id")
	userIDToRevoke, _ := strconv.ParseUint(userIDToRevokeStr, 10, 64)

	result := database.DB.Where("server_id = ? AND user_id = ?", serverID, userIDToRevoke).Delete(&models.ServerAccess{})
	if result.Error != nil || result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "access entry not found or could not be deleted"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "access revoked successfully"})
}
