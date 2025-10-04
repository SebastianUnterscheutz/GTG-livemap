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

// Init initialisiert den Cache-Client.
func Init() {
	RDB = worker.RDB // Wir verwenden die gleiche Verbindung wie der Worker
}

// Set speichert einen Wert im Cache.
func Set(key string, value interface{}, expiration time.Duration) error {
	// Konvertiere den Wert in JSON, da Redis am besten mit Strings arbeitet.
	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return RDB.Set(context.Background(), key, jsonData, expiration).Err()
}

// Get retrieves a value from the cache and decodes it into the passed target object.
func Get(key string, dest interface{}) error {
	val, err := RDB.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return err // Wichtig: Gib den redis.Nil-Fehler weiter, damit wir wissen, dass der Key nicht existiert.
	} else if err != nil {
		return err
	}

	// Decode the JSON string back into the Go object.
	return json.Unmarshal([]byte(val), dest)
}

// Delete removes a key from the cache.
func Delete(key string) error {
	return RDB.Del(context.Background(), key).Err()
}

// RecentTimestampsCacheKey is the public key for the timeline cache.
const RecentTimestampsCacheKey = "timeline:recent:%s" // %s wird durch die serverID ersetzt

// StartRecentTimestampsWorker is a background process that keeps the cache up to date.
func StartRecentTimestampsWorker(ctx context.Context) {
	log.Println("Starting Recent Timestamps Cache Worker...")
	ticker := time.NewTicker(5 * time.Second) // Alle 5 Sekunden aktualisieren

	// Execute the function once immediately without waiting.
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
	// Find all servers that have sent data in the last hour.
	oneHourAgo := time.Now().UTC().Add(-1 * time.Hour)
	database.DB.Model(&models.Server{}).
		Joins("JOIN player_positions ON player_positions.server_id = servers.id AND player_positions.event_timestamp > ?", oneHourAgo).
		Group("servers.id").
		Find(&activeServers)

	for _, server := range activeServers {
		// ★★★ Hier verwenden wir die exakt gleiche Logik wie im GetTimestampsHandler ★★★
		threeHoursAgo := time.Now().UTC().Add(-3 * time.Hour)
		var timestamps []int64

		database.DB.Model(&models.PlayerPosition{}).
			Where("server_id = ? AND event_timestamp >= ?", server.ID, threeHoursAgo).
			Order("event_timestamp asc").
			Pluck("DISTINCT FLOOR(UNIX_TIMESTAMP(event_timestamp))", &timestamps)

		if len(timestamps) > 0 {
			// Wir speichern die Daten als einfaches JSON-Array.
			cacheKey := fmt.Sprintf(RecentTimestampsCacheKey, server.ID.String())
			// Haltbarkeit von 10 Minuten. Wenn ein Server inaktiv wird, verschwindet der Cache von selbst.
			Set(cacheKey, timestamps, 10*time.Minute)
		}
	}
}

const RecentDamageEventTimestampsCacheKey = "damagetimeline:recent:%s" // %s wird durch die serverID ersetzt

// ★★★ NEU: Worker für Damage-Event-Timestamps ★★★
// StartDamageEventTimestampsWorker is a background process that keeps the event cache up to date.
func StartDamageEventTimestampsWorker(ctx context.Context) {
	log.Println("Starting Recent Damage Event Timestamps Cache Worker...")
	ticker := time.NewTicker(5 * time.Second) // Gleiches Intervall wie der andere Worker

	// Execute once immediately
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

// ★★★ NEU: Update-Funktion für den Damage-Event-Cache ★★★
func updateDamageEventTimestampsCache() {
	// We can reuse the same logic for finding active servers.
	// A server that sends position data is a good indicator of general activity.
	var activeServers []models.Server
	oneHourAgo := time.Now().UTC().Add(-1 * time.Hour)
	database.DB.Model(&models.Server{}).
		Joins("JOIN player_positions ON player_positions.server_id = servers.id AND player_positions.event_timestamp > ?", oneHourAgo).
		Group("servers.id").
		Find(&activeServers)

	for _, server := range activeServers {
		threeHoursAgo := time.Now().UTC().Add(-3 * time.Hour)
		var timestamps []int64

		// Holen und in Unix umwandeln. Die Query ist identisch, nur auf die "damage_events"-Tabelle bezogen.
		database.DB.Model(&models.DamageEvent{}).
			Where("server_id = ? AND event_timestamp >= ?", server.ID, threeHoursAgo).
			Order("event_timestamp asc").
			Pluck("DISTINCT FLOOR(UNIX_TIMESTAMP(event_timestamp))", &timestamps)

		if len(timestamps) > 0 {
			// We use the new cache key.
			cacheKey := fmt.Sprintf(RecentDamageEventTimestampsCacheKey, server.ID.String())
			// Haltbarkeit von 10 Minuten.
			Set(cacheKey, timestamps, 10*time.Minute)
		}
	}
}
