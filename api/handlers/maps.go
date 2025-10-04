// api/handlers/maps.go
package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UpdateMapConfigHandler updates the initial view settings of a map.
func UpdateMapConfigHandler(c *gin.Context) {
	mapIDStr := c.Param("id")
	mapID, _ := strconv.ParseUint(mapIDStr, 10, 64)

	var req struct {
		Lat  float64 `json:"lat" binding:"required"`
		Lng  float64 `json:"lng" binding:"required"`
		Zoom int     `json:"zoom" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// Update command to the database
	result := database.DB.Model(&models.MapConfig{}).Where("id = ?", mapID).Updates(map[string]interface{}{
		"initial_view_lat":  &req.Lat,  // Pass pointer
		"initial_view_lng":  &req.Lng,  // Pass pointer
		"initial_view_zoom": &req.Zoom, // Pass pointer
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update map config"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "map config not found or no changes made"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "map default view updated successfully"})
}
