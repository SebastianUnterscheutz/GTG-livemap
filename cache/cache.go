package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"gtglivemap/database"
	"gtglivemap/models"
	"gtglivemap/worker" // Wiederverwendung des Redis-Clients
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

func Init() {
	RDB = worker.RDB
}

func Set(key string, value interface{}, expiration time.Duration) error {
	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return RDB.Set(context.Background(), key, jsonData, expiration).Err()
}

func Get(key string, dest interface{}) error {
	val, err := RDB.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return err
	} else if err != nil {
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

// Delete removes a key from the cache.
func Delete(key string) error {
	return RDB.Del(context.Background(), key).Err()
}

// RecentTimestampsCacheKey is the public key for the timeline cache.
const RecentTimestampsCacheKey = "timeline:recent:%s"

// StartRecentTimestampsWorker is a background process that keeps the cache up to date.
func StartRecentTimestampsWorker(ctx context.Context) {
	log.Println("Starting Recent Timestamps Cache Worker...")
	ticker := time.NewTicker(5 * time.Second)

	updateRecentTimestampsCache()

	for {
		select {
		case <-ticker.C:
			updateRecentTimestampsCache()
		case <-ctx.Done():
			log.Println("Stopping Recent Timestamps Cache Worker...")
			ticker.Stop()
			return
		}
	}
}

// updateRecentTimestampsCache refreshes the timeline cache for recently active servers.
func updateRecentTimestampsCache() {
	var activeServers []models.Server
	oneHourAgo := time.Now().UTC().Add(-1 * time.Hour)
	database.DB.Model(&models.Server{}).
		Joins("JOIN player_positions ON player_positions.server_id = servers.id AND player_positions.event_timestamp > ?", oneHourAgo).
		Group("servers.id").
		Find(&activeServers)

	for _, server := range activeServers {
		threeHoursAgo := time.Now().UTC().Add(-3 * time.Hour)
		var timestamps []int64

		database.DB.Model(&models.PlayerPosition{}).
			Where("server_id = ? AND event_timestamp >= ?", server.ID, threeHoursAgo).
			Order("event_timestamp asc").
			Pluck("DISTINCT FLOOR(EXTRACT(EPOCH FROM event_timestamp))", &timestamps)

		if len(timestamps) > 0 {
			cacheKey := fmt.Sprintf(RecentTimestampsCacheKey, server.ID.String())
			Set(cacheKey, timestamps, 10*time.Minute)
		}
	}
}

const RecentDamageEventTimestampsCacheKey = "damagetimeline:recent:%s" // %s wird durch die serverID ersetzt

// StartDamageEventTimestampsWorker is a background process that keeps the event cache up to date.
func StartDamageEventTimestampsWorker(ctx context.Context) {
	log.Println("Starting Recent Damage Event Timestamps Cache Worker...")
	ticker := time.NewTicker(5 * time.Second) // Gleiches Intervall wie der andere Worker

	updateDamageEventTimestampsCache()

	for {
		select {
		case <-ticker.C:
			updateDamageEventTimestampsCache()
		case <-ctx.Done():
			log.Println("Stopping Recent Damage Event Timestamps Cache Worker...")
			ticker.Stop()
			return
		}
	}
}

func updateDamageEventTimestampsCache() {
	var activeServers []models.Server
	oneHourAgo := time.Now().UTC().Add(-1 * time.Hour)
	database.DB.Model(&models.Server{}).
		Joins("JOIN player_positions ON player_positions.server_id = servers.id AND player_positions.event_timestamp > ?", oneHourAgo).
		Group("servers.id").
		Find(&activeServers)

	for _, server := range activeServers {
		threeHoursAgo := time.Now().UTC().Add(-3 * time.Hour)
		var timestamps []int64

		database.DB.Model(&models.DamageEvent{}).
			Where("server_id = ? AND event_timestamp >= ?", server.ID, threeHoursAgo).
			Order("event_timestamp asc").
			Pluck("DISTINCT FLOOR(EXTRACT(EPOCH FROM event_timestamp))", &timestamps)

		if len(timestamps) > 0 {
			cacheKey := fmt.Sprintf(RecentDamageEventTimestampsCacheKey, server.ID.String())
			Set(cacheKey, timestamps, 10*time.Minute)
		}
	}
}
