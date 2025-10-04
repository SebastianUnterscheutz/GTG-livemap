package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

// AdminGetDemoSettingsHandler holt die aktuellen Demo-Einstellungen.
func AdminGetDemoSettingsHandler(c *gin.Context) {
	var demoServerID, demoTimestamp models.SystemSetting
	database.DB.First(&demoServerID, "key = ?", "demo_server_id")
	database.DB.First(&demoTimestamp, "key = ?", "demo_timestamp")

	c.JSON(http.StatusOK, gin.H{
		"server_id": demoServerID.Value,
		"timestamp": demoTimestamp.Value,
	})
}

// AdminSetDemoSettingsHandler setzt den Demo-Server und den Zeitpunkt.
func AdminSetDemoSettingsHandler(c *gin.Context) {
	var req struct {
		ServerID  string `json:"server_id" binding:"required"`
		Timestamp string `json:"timestamp" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: 'server_id' and 'timestamp' are required."})
		return
	}

	// Validierung, dass der Timestamp eine Zahl ist
	if _, err := strconv.ParseInt(req.Timestamp, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid timestamp format, must be a Unix timestamp."})
		return
	}

	settingsToSave := []models.SystemSetting{
		{Key: "demo_server_id", Value: req.ServerID},
		{Key: "demo_timestamp", Value: req.Timestamp},
	}

	// Upsert: Creates entries if they don't exist, or updates them.
	if err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&settingsToSave).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save demo settings."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Demo settings updated successfully."})
}

func AdminUpdateMapCalibrationHandler(c *gin.Context) {
	mapIDStr := c.Param("id")
	mapID, _ := strconv.ParseUint(mapIDStr, 10, 64)

	var req struct {
		ScaleY  *float64 `json:"scaleY" binding:"required"`
		ScaleX  *float64 `json:"scaleX" binding:"required"`
		OffsetX *float64 `json:"offsetX" binding:"required"`
		OffsetY *float64 `json:"offsetY" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	updates := map[string]interface{}{
		"scale_x":  req.ScaleX,
		"scale_y":  req.ScaleY,
		"offset_x": req.OffsetX,
		"offset_y": req.OffsetY,
	}

	result := database.DB.Model(&models.MapConfig{}).Where("id = ?", mapID).Updates(updates)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update map calibration."})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Map config not found."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Map calibrated successfully."})
}
