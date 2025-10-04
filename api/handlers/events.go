// api/handlers/events.go
package handlers

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func PostDamageEventsHandler(c *gin.Context) {
	serverID_iface, exists := c.Get("server_id")
	if !exists {
		// This case should never occur due to the middleware
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_id context is missing"})
		return
	}
	serverID, ok := serverID_iface.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_id context has wrong type"})
		return
	}

	var payload models.DamagePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format: " + err.Error()})
		return
	}

	var allEventsToSave []models.DamageEvent
	for _, eventData := range payload.Events {
		dbRecord := models.DamageEvent{
			ServerID:       serverID,
			EventTimestamp: time.Unix(eventData.Timestamp, 0),
			KillerGUID:     eventData.KillerGUID,
			VictimGUID:     eventData.VictimGUID,
			WeaponName:     eventData.WeaponName,
			DamageAmount:   eventData.DamageAmount,
			Distance:       eventData.Distance,
			HitZone:        eventData.HitZone,
			IsFriendlyFire: eventData.IsFriendlyFire,
		}
		allEventsToSave = append(allEventsToSave, dbRecord)
	}

	if len(allEventsToSave) > 0 {
		if err := database.DB.Create(&allEventsToSave).Error; err != nil {
			log.Printf("FEHLER (ServerID %s): Konnte Schadens-Events nicht in DB speichern: %v", serverID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error while saving damage events."})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Damage events received successfully",
		"records_saved": len(allEventsToSave),
	})
}
