// worker/scheduler.go
package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"gtglivemap/database"
	"gtglivemap/models"

	"github.com/go-co-op/gocron"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9" // War go-redis/redis/v8
)

const JobQueueName = "gtg_log_fetch_jobs"

var RDB *redis.Client

type LogFetchJob struct {
	ServerID uuid.UUID `json:"server_id"`
}

func InitScheduler() *gocron.Scheduler {
	s := gocron.NewScheduler(time.UTC)

	s.Every(1).Minute().Do(scheduleLogFetchingTasks)

	s.Every(1).Day().At("03:00").Do(scheduleCleanupTask)

	log.Println("Cron scheduler (Producer) configured with [fetch_logs] and [cleanup_data] tasks.")
	return s
}

func scheduleCleanupTask() {
	log.Println("Scheduler (Producer): Enqueuing daily data cleanup job.")

	job := Job{Type: "cleanup_data"}
	jobBytes, err := json.Marshal(job)
	if err != nil {
		log.Printf("ERROR marshaling cleanup job: %v", err)
		return
	}

	err = RDB.LPush(context.Background(), JobQueueName, jobBytes).Err()
	if err != nil {
		log.Printf("ERROR enqueuing cleanup job: %v", err)
	}
}

func scheduleLogFetchingTasks() {
	log.Println("Scheduler (Producer): Looking for servers to enqueue...")

	var serversToProcess []models.Server
	if err := database.DB.Where("log_source_type IN (?)", []string{"ftp", "sftp"}).Find(&serversToProcess).Error; err != nil {
		log.Printf("ERROR: Could not fetch servers for scheduler: %v", err)
		return
	}

	if len(serversToProcess) == 0 {
		log.Println("Scheduler (Producer): No servers to enqueue.")
		return
	}

	for _, server := range serversToProcess {
		job := Job{
			Type:     "fetch_logs",
			ServerID: server.ID,
		}

		jobBytes, err := json.Marshal(job)
		if err != nil {
			log.Printf("ERROR marshaling job for server %s: %v", server.ID, err)
			continue
		}

		err = RDB.LPush(context.Background(), JobQueueName, jobBytes).Err()
		if err != nil {
			log.Printf("ERROR enqueuing job for server %s: %v", server.ID, err)
		} else {
			log.Printf("Scheduler (Producer): Enqueued 'fetch_logs' job for server ID %s", server.ID.String())
		}
	}
}
