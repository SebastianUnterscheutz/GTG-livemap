// in api/handlers/server_status.go (oder eine passende andere Datei)
package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ServerStatusResponse definiert die Daten, die wir für das Frontend-Feedback benötigen.
type ServerStatusResponse struct {
	LastPositionReceivedAt *time.Time `json:"last_position_received_at"`
	PositionRecordCount    int64      `json:"position_record_count"`
	LastEventReceivedAt    *time.Time `json:"last_event_received_at"`
	EventRecordCount       int64      `json:"event_record_count"`
}

// GetServerStatusHandler prüft den Zustand eines Servers und gibt Feedback-Daten zurück.
func GetServerStatusHandler(c *gin.Context) {
	// Die Middleware stellt sicher, dass der User Besitzer oder Admin ist.
	serverIDStr := c.Param("id")
	serverID, _ := uuid.Parse(serverIDStr)

	var response ServerStatusResponse

	// 1. Prüfe Positionsdaten (API Push)
	var latestPosition models.PlayerPosition
	err := database.DB.Where("server_id = ?", serverID).Order("event_timestamp desc").First(&latestPosition).Error
	if err == nil { // Wenn ein Eintrag gefunden wurde
		response.LastPositionReceivedAt = &latestPosition.EventTimestamp
	}
	database.DB.Model(&models.PlayerPosition{}).Where("server_id = ?", serverID).Count(&response.PositionRecordCount)

	// 2. Prüfe Event-Daten (FTP/SFTP Pull)
	var latestEvent models.DamageEvent
	err = database.DB.Where("server_id = ?", serverID).Order("event_timestamp desc").First(&latestEvent).Error
	if err == nil { // Wenn ein Eintrag gefunden wurde
		response.LastEventReceivedAt = &latestEvent.EventTimestamp
	}
	database.DB.Model(&models.DamageEvent{}).Where("server_id = ?", serverID).Count(&response.EventRecordCount)

	c.JSON(http.StatusOK, response)
}
