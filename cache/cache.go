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

// Get ruft einen Wert aus dem Cache ab und dekodiert ihn in das übergebene Zielobjekt.
func Get(key string, dest interface{}) error {
	val, err := RDB.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return err // Wichtig: Gib den redis.Nil-Fehler weiter, damit wir wissen, dass der Key nicht existiert.
	} else if err != nil {
		return err
	}

	// Dekodiere den JSON-String zurück in das Go-Objekt.
	return json.Unmarshal([]byte(val), dest)
}

// Delete entfernt einen Schlüssel aus dem Cache.
func Delete(key string) error {
	return RDB.Del(context.Background(), key).Err()
}

// RecentTimestampsCacheKey ist der öffentliche Schlüssel für den Timeline-Cache.
const RecentTimestampsCacheKey = "timeline:recent:%s" // %s wird durch die serverID ersetzt

// StartRecentTimestampsWorker ist ein Hintergrundprozess, der den Cache aktuell hält.
func StartRecentTimestampsWorker(ctx context.Context) {
	log.Println("Starting Recent Timestamps Cache Worker...")
	ticker := time.NewTicker(5 * time.Second) // Alle 5 Sekunden aktualisieren

	// Führe die Funktion sofort einmal aus, ohne zu warten.
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

func updateRecentTimestampsCache() {
	var activeServers []models.Server
	// Finde alle Server, die in der letzten Stunde Daten gesendet haben.
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
// StartDamageEventTimestampsWorker ist ein Hintergrundprozess, der den Event-Cache aktuell hält.
func StartDamageEventTimestampsWorker(ctx context.Context) {
	log.Println("Starting Recent Damage Event Timestamps Cache Worker...")
	ticker := time.NewTicker(5 * time.Second) // Gleiches Intervall wie der andere Worker

	// Sofort einmal ausführen
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
	// Wir können die gleiche Logik zur Findung aktiver Server wiederverwenden.
	// Ein Server, der Positionsdaten sendet, ist ein guter Indikator für allgemeine Aktivität.
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
			// Wir verwenden den neuen Cache-Schlüssel.
			cacheKey := fmt.Sprintf(RecentDamageEventTimestampsCacheKey, server.ID.String())
			// Haltbarkeit von 10 Minuten.
			Set(cacheKey, timestamps, 10*time.Minute)
		}
	}
}
