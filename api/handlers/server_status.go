package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ServerStatusResponse defines the data we need for frontend feedback.
type ServerStatusResponse struct {
	LastPositionReceivedAt *time.Time `json:"last_position_received_at"`
	PositionRecordCount    int64      `json:"position_record_count"`
	LastEventReceivedAt    *time.Time `json:"last_event_received_at"`
	EventRecordCount       int64      `json:"event_record_count"`
}

// GetServerStatusHandler checks the status of a server and returns feedback data.
func GetServerStatusHandler(c *gin.Context) {
	// The middleware ensures that the user is the owner or an admin.
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	var response ServerStatusResponse

	// 1. Check position data (API Push)
	var latestPosition models.PlayerPosition
	err := database.DB.Where("server_id = ?", serverID).Order("event_timestamp desc").First(&latestPosition).Error
	if err == nil { // If an entry was found
		response.LastPositionReceivedAt = &latestPosition.EventTimestamp
	}
	database.DB.Model(&models.PlayerPosition{}).Where("server_id = ?", serverID).Count(&response.PositionRecordCount)

	// 2. Check event data (FTP/SFTP Pull)
	var latestEvent models.DamageEvent
	err = database.DB.Where("server_id = ?", serverID).Order("event_timestamp desc").First(&latestEvent).Error
	if err == nil { // If an entry was found
		response.LastEventReceivedAt = &latestEvent.EventTimestamp
	}
	database.DB.Model(&models.DamageEvent{}).Where("server_id = ?", serverID).Count(&response.EventRecordCount)

	c.JSON(http.StatusOK, response)
}
