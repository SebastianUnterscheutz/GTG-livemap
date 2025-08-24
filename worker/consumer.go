package worker

import (
	"context"
	"encoding/json"
	"gtglivemap/database"
	"gtglivemap/models"
	"gtglivemap/pkg/logfetcher"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Job struct {
	Type     string    `json:"type"`
	ServerID uuid.UUID `json:"server_id,omitempty"`
}

func StartJobConsumer(ctx context.Context) {
	log.Println("Job Consumer started. Waiting for jobs...")
	for {
		select {
		case <-ctx.Done():
			log.Println("Job Consumer received shutdown signal, stopping.")
			return

		default:
			result, err := RDB.BRPop(ctx, 1*time.Second, JobQueueName).Result()

			if err == redis.Nil {
				// Das ist kein Fehler, sondern ein normaler Timeout. Es war einfach kein Job da.
				// Die Schleife beginnt von vorn.
				continue
			}
			if err != nil {
				log.Printf("ERROR fetching job from Redis queue: %v. Retrying in 5 seconds...", err)
				time.Sleep(5 * time.Second)
				continue
			}

			jobData := []byte(result[1])
			var job Job

			if err := json.Unmarshal(jobData, &job); err != nil {
				log.Printf("ERROR unmarshaling job data: %v. Data: %s", err, string(jobData))
				continue
			}

			switch job.Type {
			case "fetch_logs":
				if job.ServerID == uuid.New() {
					log.Printf("WARNING: 'fetch_logs' job received without ServerID. Skipping.")
					continue
				}
				log.Printf("Job Consumer: Received 'fetch_logs' job for server ID %s", job.ServerID)

				var server models.Server
				if err := database.DB.First(&server, job.ServerID).Error; err != nil {
					log.Printf("ERROR: Could not find server with ID %s for job: %v", job.ServerID, err)
					continue
				}

				logfetcher.ProcessServerLogs(server)

			case "cleanup_data":
				log.Printf("Job Consumer: Received 'cleanup_data' job.")
				logfetcher.RunDataCleanup()

			default:
				log.Printf("WARNING: Unknown job type received: '%s'", job.Type)
			}
		}
	}
}
