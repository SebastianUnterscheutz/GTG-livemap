// pkg/logfetcher/cleanup.go
package logfetcher

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"time"

	"gorm.io/gorm"
)

func RunDataCleanup() {
	log.Println("Cleanup Worker: Starting data cleanup process for all servers.")

	var servers []models.Server
	if err := database.DB.Find(&servers).Error; err != nil {
		log.Printf("Cleanup ERROR: Could not fetch servers: %v", err)
		return
	}

	var totalDeletedPos, totalDeletedEvents int64

	for _, server := range servers {

		cutoffDate := time.Now().AddDate(0, 0, -server.MaxStorageDays)

		log.Printf(" - Cleaning server %s (%s), deleting records older than %s (%d days retention)",
			server.ID, server.Name, cutoffDate.Format("2006-01-02"), server.MaxStorageDays)

		err := database.DB.Transaction(func(tx *gorm.DB) error {
			resPos := tx.Where("server_id = ? AND event_timestamp < ?", server.ID, cutoffDate).Delete(&models.PlayerPosition{})
			if resPos.Error != nil {
				return resPos.Error
			}
			totalDeletedPos += resPos.RowsAffected

			resEvents := tx.Where("server_id = ? AND event_timestamp < ?", server.ID, cutoffDate).Delete(&models.DamageEvent{})
			if resEvents.Error != nil {
				return resEvents.Error
			}
			totalDeletedEvents += resEvents.RowsAffected

			return nil
		})

		if err != nil {
			log.Printf("Cleanup ERROR for server %d: %v", server.ID, err)
		}
	}

	log.Printf("Cleanup Worker: Finished. Deleted %d position records and %d event records.", totalDeletedPos, totalDeletedEvents)
}
