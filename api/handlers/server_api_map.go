package handlers

import (
	"errors"
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIGetCurrentMapHandler returns the current map configuration of the server.
func APIGetCurrentMapHandler(c *gin.Context) {
	serverID_iface, _ := c.Get("server_id")
	serverID := serverID_iface.(uuid.UUID)

	var server models.Server
	if err := database.DB.Preload("MapConfig").First(&server, "id = ?", serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

	server.MapConfig.TilesURL = rewriteTilesURL(server.MapConfig.TilesURL)

	c.JSON(http.StatusOK, server.MapConfig)
}

// APISetCurrentMapHandler changes the map for the current server.
func APISetCurrentMapHandler(c *gin.Context) {
	serverID_iface, _ := c.Get("server_id")
	serverID := serverID_iface.(uuid.UUID)

	var req struct {
		MapID uint `json:"map_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: map_id (number) is required"})
		return
	}

	// Validation: Check if the map ID even exists.
	var mapConfig models.MapConfig
	if err := database.DB.First(&mapConfig, req.MapID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Map with the specified map_id does not exist."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error while checking map."})
		return
	}

	// Update the server
	result := database.DB.Model(&models.Server{}).Where("id = ?", serverID).Update("map_config_id", req.MapID)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update server map"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Server map updated successfully"})
}
