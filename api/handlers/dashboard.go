// in api/handlers/dashboard.go
package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetDashboardServerStatusesHandler provides status timestamps for the user's servers
// by reading pre-calculated values directly from the servers table for high performance.
func GetDashboardServerStatusesHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDUint64 := userID.(uint64)

	var user models.User
	database.DB.First(&user, userIDUint64)

	var accessibleServers []models.Server

	// 1. Fetch all server objects the user has access to
	if user.AccountType == "admin" {
		// Admin gets all servers
		database.DB.Find(&accessibleServers)
	} else {
		// Fetch owned servers
		var ownedServers []models.Server
		database.DB.Where("owner_id = ?", userIDUint64).Find(&ownedServers)
		accessibleServers = append(accessibleServers, ownedServers...)

		// Fetch shared servers
		var sharedAccesses []models.ServerAccess
		database.DB.Preload("Server").Where("user_id = ?", userIDUint64).Find(&sharedAccesses)

		// Add shared servers, avoiding duplicates if a user has access to their own server
		ownedServerIDs := make(map[uuid.UUID]bool)
		for _, s := range ownedServers {
			ownedServerIDs[s.ID] = true
		}
		for _, access := range sharedAccesses {
			if _, isOwner := ownedServerIDs[access.ServerID]; !isOwner {
				accessibleServers = append(accessibleServers, access.Server)
			}
		}
	}

	if len(accessibleServers) == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	type StatusResponse struct {
		LastPositionReceivedAt *time.Time `json:"last_position_received_at"`
		LastEventReceivedAt    *time.Time `json:"last_event_received_at"`
	}
	response := make(map[uuid.UUID]StatusResponse)

	// 3. Build the response directly from the server objects' timestamp fields
	for _, server := range accessibleServers {
		status := StatusResponse{
			LastPositionReceivedAt: server.LastProcessedPositionTimestamp,
		}

		// Determine the most recent event timestamp between damage and kill logs
		var latestEventTime *time.Time
		if server.LastProcessedDamageTimestamp != nil && server.LastProcessedKillTimestamp != nil {
			if server.LastProcessedDamageTimestamp.After(*server.LastProcessedKillTimestamp) {
				latestEventTime = server.LastProcessedDamageTimestamp
			} else {
				latestEventTime = server.LastProcessedKillTimestamp
			}
		} else if server.LastProcessedDamageTimestamp != nil {
			latestEventTime = server.LastProcessedDamageTimestamp
		} else {
			latestEventTime = server.LastProcessedKillTimestamp
		}
		status.LastEventReceivedAt = latestEventTime

		response[server.ID] = status
	}

	c.JSON(http.StatusOK, response)
}
