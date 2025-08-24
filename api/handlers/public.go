package handlers

import (
	"fmt"
	"gtglivemap/cache"
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func GetPublicServersHandler(c *gin.Context) {
	var publicServers []models.Server

	result := database.DB.Preload("MapConfig").Where("is_public = ?", true).Find(&publicServers)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}

	type PublicServerResponse struct {
		ID        uuid.UUID        `json:"id"`
		Name      string           `json:"name"`
		IsListed  bool             `json:"is_listed"`
		MapConfig models.MapConfig `json:"map_config"`
	}

	response := make([]PublicServerResponse, len(publicServers))
	for i, server := range publicServers {
		server.MapConfig.TilesURL = rewriteTilesURL(server.MapConfig.TilesURL)
		response[i] = PublicServerResponse{
			ID:        server.ID,
			Name:      server.Name,
			IsListed:  server.IsListed,
			MapConfig: server.MapConfig,
		}
	}

	c.JSON(http.StatusOK, response)
}

const MaxTimestamps = 10801

// GetTimestampsHandler liefert die Zeitstempel für die Timeline auf der Karte.
// Wenn mehr als MaxTimestamps gefunden werden, wird die Datenmenge intelligent reduziert (Downsampling).
func GetTimestampsHandler(c *gin.Context) {
	// 1. Parameter auslesen und validieren
	serverIDStr := c.Param("server_id")
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID format"})
		return
	}

	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return
	}

	query := database.DB.Model(&models.PlayerPosition{}).Where("server_id = ?", serverID)
	fromStr := c.Query("from")
	toStr := c.Query("to")

	if fromStr == "" && toStr == "" {
		cacheKey := fmt.Sprintf(cache.RecentTimestampsCacheKey, serverID.String())
		var cachedTimestamps []int64
		if err := cache.Get(cacheKey, &cachedTimestamps); err == nil && len(cachedTimestamps) > 0 {
			// Cache-Treffer!
			c.JSON(http.StatusOK, gin.H{
				"timestamps":      cachedTimestamps,
				"was_downsampled": false,
			})
			return
		}
		// Falls Cache leer, weiter mit Datenbankabfrage für die letzten 3 Stunden
		threeHoursAgo := time.Now().UTC().Add(-3 * time.Hour)
		query = query.Where("event_timestamp >= ?", threeHoursAgo)
	} else {
		// Bei Vorhandensein von FROM/TO, die Abfrage direkt auf die DB anwenden.
		fromTs, _ := strconv.ParseInt(fromStr, 10, 64)
		toTs, _ := strconv.ParseInt(toStr, 10, 64)
		query = query.Where("event_timestamp BETWEEN ? AND ?", time.Unix(fromTs, 0), time.Unix(toTs, 0))
	}
	var timestamps []time.Time
	var wasDownsampled bool = false

	var count int64
	countQuery := query.Session(&gorm.Session{})
	if err := countQuery.Distinct("event_timestamp").Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count timestamps"})
		return
	}

	if count <= MaxTimestamps {
		log.Printf("Found %d timestamps (<= %d) for server %s, returning all.", count, MaxTimestamps, serverID)
		if err := query.Distinct("event_timestamp").Order("event_timestamp asc").Pluck("event_timestamp", &timestamps).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query all timestamps"})
			return
		}
	} else {
		wasDownsampled = true
		log.Printf("Found %d timestamps (> %d) for server %s, performing downsampling.", count, MaxTimestamps, serverID)

		subQuery := query.Distinct("event_timestamp").Order("event_timestamp asc")

		rawSQL := fmt.Sprintf(
			`SELECT event_timestamp FROM
             (SELECT event_timestamp, ROW_NUMBER() OVER (ORDER BY event_timestamp) as rn, COUNT(*) OVER () as total_count
              FROM (?) as t) as ranked
           WHERE MOD(rn, CEIL(total_count / ?)) = 1 OR rn = total_count`,
		)

		if err := database.DB.Raw(rawSQL, subQuery, float64(MaxTimestamps)).Scan(&timestamps).Error; err != nil {
			log.Printf("ERROR downsampling timestamps for server %s: %v", serverID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to downsample timestamps"})
			return
		}
	}

	// 5. Ergebnis in das gewünschte Format umwandeln und senden
	unixTimestamps := make([]int64, len(timestamps))
	for i, ts := range timestamps {
		unixTimestamps[i] = ts.Unix()
	}

	c.JSON(http.StatusOK, gin.H{
		"timestamps":      unixTimestamps,
		"was_downsampled": wasDownsampled,
	})
}

// in api/handlers/public_routes.go
func GetPositionsByTimeHandler(c *gin.Context) {
	serverIDStr := c.Param("server_id")
	timestampStr := c.Param("timestamp")
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}
	targetTime, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid timestamp format."})
		return
	}
	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return
	}

	// Die Response-Struktur bleibt unverändert.
	type FactionResponse struct {
		Name   string  `json:"name"`
		ColorR float64 `json:"colorR"`
		ColorG float64 `json:"colorG"`
		ColorB float64 `json:"colorB"`
	}
	type PositionResponse struct {
		PlayerGUID     string          `json:"playerGuid"`
		EventTimestamp int64           `json:"eventTimestamp"`
		AbsolutePosX   float64         `json:"absolutePosX"`
		AbsolutePosZ   float64         `json:"absolutePosZ"`
		RotationX      float64         `json:"rotationX"`
		InVehicle      bool            `json:"inVehicle"`
		Faction        FactionResponse `json:"faction"`
	}

	cacheKey := fmt.Sprintf("positions:%s:%s", serverIDStr, timestampStr)
	var cachedResponse []PositionResponse // Definiere die Zielstruktur

	if err := cache.Get(cacheKey, &cachedResponse); err == nil {
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	const activityWindow = 2 * time.Second
	windowStart := targetTime.Add(-activityWindow)

	var positions []models.PlayerPosition

	// Die Unterabfrage wird angepasst, um NUR Positionen im definierten Zeitfenster zu berücksichtigen.
	subQuery := database.DB.Model(&models.PlayerPosition{}).
		Select("MAX(id)").
		Where("server_id = ? AND event_timestamp BETWEEN ? AND ?", serverID, windowStart, targetTime).
		Group("player_guid")

	// Die Hauptabfrage bleibt gleich, sie holt die Daten für die in der subQuery gefundenen IDs.
	err = database.DB.Preload("Faction").Where("id IN (?)", subQuery).Find(&positions).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed for positions"})
		return
	}

	response := make([]PositionResponse, len(positions))
	for i, pos := range positions {
		response[i] = PositionResponse{
			PlayerGUID:     pos.PlayerGUID,
			EventTimestamp: pos.EventTimestamp.Unix(),
			AbsolutePosX:   pos.AbsolutePosX,
			AbsolutePosZ:   pos.AbsolutePosZ,
			RotationX:      pos.RotationX,
			InVehicle:      pos.InVehicle,
			Faction: FactionResponse{
				Name:   pos.Faction.Name,
				ColorR: pos.Faction.ColorR,
				ColorG: pos.Faction.ColorG,
				ColorB: pos.Faction.ColorB,
			},
		}
	}
	cache.Set(cacheKey, response, 10*time.Minute)
	c.JSON(http.StatusOK, response)
}

func GetMapConfigsHandler(c *gin.Context) {
	var maps []models.MapConfig
	if err := database.DB.Find(&maps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database query failed for map configs"})
		return
	}

	for i, _ := range maps {
		maps[i].TilesURL = rewriteTilesURL(maps[i].TilesURL)
	}

	c.JSON(http.StatusOK, maps)
}

// GetDamageEventsByTimeHandler liefert Events und die zugehörigen Positionen von Killer und Victim.
func GetDamageEventsByTimeHandler(c *gin.Context) {
	serverIDStr := c.Param("server_id")
	timestampStr := c.Param("timestamp")

	serverID, _ := uuid.Parse(serverIDStr)
	targetTime, _ := time.Parse(time.RFC3339Nano, timestampStr)

	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return // Wichtig: Bricht die Ausführung ab, wenn kein Zugriff besteht.
	}

	// Wir definieren ein kleines Zeitfenster um den Abfragezeitpunkt,
	// um Events zu fangen, die knapp davor passiert sind.
	timeWindowStart := targetTime.Add(-5 * time.Second) // 5 Sekunden Fenster

	var eventsInWindow []models.DamageEvent
	database.DB.Where("server_id = ? AND event_timestamp BETWEEN ? AND ?", serverID, timeWindowStart, targetTime).Find(&eventsInWindow)

	if len(eventsInWindow) == 0 {
		c.JSON(http.StatusOK, []interface{}{}) // Leeres Array zurückgeben
		return
	}

	// Dies wird die komplexe Antwortstruktur sein, die Ihr Frontend erwartet hat.
	type Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	}
	type EventResponse struct {
		Event          models.DamageEvent `json:"event"`
		KillerPosition *Position          `json:"killerPosition"`
		VictimPosition *Position          `json:"victimPosition"`
	}

	response := make([]EventResponse, 0)

	// Helferfunktion, um die letzte Position eines Spielers zu finden.
	findLastPosition := func(guid string) *Position {
		var pos models.PlayerPosition
		// Finde den neuesten Positions-Eintrag für diesen Spieler VOR dem Event-Zeitpunkt
		err := database.DB.Where("server_id = ? AND player_guid = ? AND event_timestamp <= ?", serverID, guid, targetTime).
			Order("event_timestamp desc").First(&pos).Error
		if err != nil {
			return nil // Spieler nicht gefunden
		}
		return &Position{X: pos.AbsolutePosX, Y: pos.AbsolutePosY, Z: pos.AbsolutePosZ}
	}

	for _, event := range eventsInWindow {
		killerPos := findLastPosition(event.KillerGUID)
		victimPos := findLastPosition(event.VictimGUID)

		// Wir fügen das Event nur hinzu, wenn wir BEIDE Positionen finden konnten.
		if killerPos != nil && victimPos != nil {
			response = append(response, EventResponse{
				Event:          event,
				KillerPosition: killerPos,
				VictimPosition: victimPos,
			})
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetHeatmapHandler gibt Positionsdaten im [lat, lng, intensity] Format zurück,
// das von Leaflet.heat erwartet wird.
func GetHeatmapHandler(c *gin.Context) {
	// Query-Parameter auslesen
	serverIDStr := c.Query("server_id")
	startStr := c.Query("start") // Unix-Timestamp
	endStr := c.Query("end")     // Unix-Timestamp

	if serverIDStr == "" || startStr == "" || endStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_id, start, and end parameters are required"})
		return
	}

	serverID, _ := uuid.Parse(serverIDStr)
	startTimestamp, _ := strconv.ParseInt(startStr, 10, 64)
	endTimestamp, _ := strconv.ParseInt(endStr, 10, 64)

	startTime := time.Unix(startTimestamp, 0)
	endTime := time.Unix(endTimestamp, 0)

	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return // Wichtig: Bricht die Ausführung ab, wenn kein Zugriff besteht.
	}

	var positions []models.PlayerPosition
	// Hole alle Positionsdaten im angegebenen Zeitfenster für den Server.
	// Wir wählen nur die Spalten aus, die wir brauchen, um die Abfrage schneller zu machen.
	err := database.DB.Select("absolute_pos_z", "absolute_pos_x").
		Where("server_id = ? AND event_timestamp BETWEEN ? AND ?", serverID, startTime, endTime).
		Find(&positions).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database query failed for heatmap data"})
		return
	}

	// Das Format, das Leaflet.heat benötigt: [lat, lng, intensity]
	// Lat in Leaflet's CRS.Simple ist unsere Z-Koordinate.
	// Lng ist unsere X-Koordinate.
	// Intensity ist vorerst immer 1.0 (jeder Punkt trägt gleich viel bei).
	heatmapData := make([][3]float64, len(positions))
	for i, pos := range positions {
		heatmapData[i] = [3]float64{pos.AbsolutePosZ, pos.AbsolutePosX, 1.0}
	}

	c.JSON(http.StatusOK, heatmapData)
}

// GetPlayerNamesHandler nimmt eine Liste von GUIDs und gibt eine Map von GUID->Name zurück.
func GetPlayerNamesHandler(c *gin.Context) {
	var req struct {
		GUIDs []string `json:"guids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request, expecting a 'guids' array."})
		return
	}

	if len(req.GUIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}

	var identities []models.PlayerIdentity
	database.DB.Where("guid IN ?", req.GUIDs).Find(&identities)

	nameMap := make(map[string]string)
	for _, identity := range identities {
		nameMap[identity.GUID] = identity.LastKnownName
	}
	c.JSON(http.StatusOK, nameMap)
}

func GetDamageEventTimestampsHandler(c *gin.Context) {
	serverIDStr := c.Param("server_id")
	serverID, _ := uuid.Parse(serverIDStr)

	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return
	}

	if c.Query("from") == "" && c.Query("to") == "" {
		cacheKey := fmt.Sprintf(cache.RecentDamageEventTimestampsCacheKey, serverID.String())
		var cachedTimestamps []int64
		if err := cache.Get(cacheKey, &cachedTimestamps); err == nil {
			c.JSON(http.StatusOK, cachedTimestamps)
			return
		}
	}

	query := database.DB.Model(&models.DamageEvent{}).Where("server_id = ?", serverID)

	// Unterstützung für den Zeitfilter (wichtig!)
	if fromStr := c.Query("from"); fromStr != "" {
		fromTs, _ := strconv.ParseInt(fromStr, 10, 64)
		query = query.Where("event_timestamp >= ?", time.Unix(fromTs, 0))
	}
	if toStr := c.Query("to"); toStr != "" {
		toTs, _ := strconv.ParseInt(toStr, 10, 64)
		query = query.Where("event_timestamp <= ?", time.Unix(toTs, 0))
	}

	var timestamps []time.Time
	if err := query.Order("event_timestamp asc").Pluck("event_timestamp", &timestamps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database query failed"})
		return
	}

	unixTimestamps := make([]int64, len(timestamps))
	for i, ts := range timestamps {
		unixTimestamps[i] = ts.Unix()
	}

	c.JSON(http.StatusOK, unixTimestamps)
}

// GetPlayerEventTimestampsHandler holt alle Zeitstempel von Kills/Toden für einen spezifischen Spieler.
func GetPlayerEventTimestampsHandler(c *gin.Context) {

	serverIDStr := c.Param("server_id")
	guid := c.Param("guid")

	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID format. A valid UUID is required."})
		return
	}

	if guid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player GUID is required."})
		return
	}

	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return
	}

	query := database.DB.Model(&models.DamageEvent{}).
		Where("server_id = ? AND (victim_guid = ? OR killer_guid = ?)", serverID, guid, guid)

	// 3. Zeitstempel-Filter sicher anwenden
	if fromStr := c.Query("from"); fromStr != "" {
		fromTs, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'from' timestamp. Please provide a valid Unix timestamp."})
			return
		}
		query = query.Where("event_timestamp >= ?", time.Unix(fromTs, 0))
	}

	if toStr := c.Query("to"); toStr != "" {
		toTs, err := strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'to' timestamp. Please provide a valid Unix timestamp."})
			return
		}
		query = query.Where("event_timestamp <= ?", time.Unix(toTs, 0))
	}

	var timestamps []time.Time
	// Wir verwenden Pluck, um nur die eine Spalte effizient zu laden.
	if err := query.Order("event_timestamp asc").Pluck("event_timestamp", &timestamps).Error; err != nil {
		log.Printf("ERROR (ServerID %s, GUID %s): Database query failed for player event timestamps: %v", serverID, guid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed while fetching event timestamps."})
		return
	}

	unixTimestamps := make([]int64, len(timestamps))
	for i, ts := range timestamps {
		unixTimestamps[i] = ts.Unix()
	}

	c.JSON(http.StatusOK, unixTimestamps)
}

func GetLatestPositionsHandler(c *gin.Context) {
	// 1. Parameter und Zugriff prüfen (bleibt gleich)
	serverIDStr := c.Param("server_id")
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID format"})
		return
	}
	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return
	}

	// 2. Definiere die Response-Struktur, die aus dem Cache erwartet wird
	type FactionResponse struct {
		Name   string  `json:"name"`
		ColorR float64 `json:"colorR"`
		ColorG float64 `json:"colorG"`
		ColorB float64 `json:"colorB"`
	}
	type PositionResponse struct {
		PlayerGUID     string          `json:"playerGuid"`
		EventTimestamp int64           `json:"eventTimestamp"`
		AbsolutePosX   float64         `json:"absolutePosX"`
		AbsolutePosZ   float64         `json:"absolutePosZ"`
		RotationX      float64         `json:"rotationX"`
		InVehicle      bool            `json:"inVehicle"`
		Faction        FactionResponse `json:"faction"`
	}

	cacheKey := fmt.Sprintf("latest_positions:%s", serverIDStr)
	var cachedResponse []PositionResponse

	// 3. Versuche, die Daten aus dem Cache abzurufen.
	if err := cache.Get(cacheKey, &cachedResponse); err == nil {
		// Cache-Treffer! Sende die gecachten Daten.
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	// 4. Cache Miss: Der Cache ist leer. Sende ein leeres Array.
	// Der Cache wird beim nächsten POST-Request automatisch gefüllt.
	c.JSON(http.StatusOK, []PositionResponse{})
}

func GetDamageEventsInRangeHandler(c *gin.Context) {
	// 1. Parameter auslesen und validieren
	serverIDStr := c.Param("server_id")
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID format"})
		return
	}
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "'from' and 'to' query parameters are required"})
		return
	}
	fromTs, _ := strconv.ParseInt(fromStr, 10, 64)
	toTs, _ := strconv.ParseInt(toStr, 10, 64)
	startTime := time.Unix(fromTs, 0)
	endTime := time.Unix(toTs, 0)

	// 2. Zugriffsberechtigung prüfen
	if errResponse, statusCode := CheckServerAccess(c, serverID); errResponse != nil {
		c.JSON(statusCode, errResponse)
		return
	}

	// 3. Alle relevanten Events abrufen (ohne "AI/World")
	var eventsInWindow []models.DamageEvent
	if err := database.DB.Where("server_id = ? AND event_timestamp BETWEEN ? AND ? AND killer_guid != 'AI/World' AND victim_guid != 'AI/World'",
		serverID, startTime, endTime).Order("event_timestamp asc").Find(&eventsInWindow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch damage events"})
		return
	}

	// 4. Rohe Daten direkt zurücksenden
	c.JSON(http.StatusOK, eventsInWindow)
}

func GetDemoDataHandler(c *gin.Context) {

	// 1. Lade die Einstellungen aus der DB
	var demoServerIDSetting, demoTimestampSetting models.SystemSetting
	database.DB.First(&demoServerIDSetting, "`key` = ?", "demo_server_id")
	database.DB.First(&demoTimestampSetting, "`key` = ?", "demo_timestamp")

	if demoServerIDSetting.Value == "" || demoTimestampSetting.Value == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Demo server is not configured."})
		return
	}
	serverID, _ := uuid.Parse(demoServerIDSetting.Value)

	timestamp, _ := strconv.ParseInt(demoTimestampSetting.Value, 10, 64)
	targetTime := time.Unix(timestamp, 0)

	// 2. Lade die Kartenkonfiguration des Servers
	var server models.Server
	if err := database.DB.Preload("MapConfig").First(&server, "id = ?", serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Configured demo server not found."})
		return
	}

	cacheKey := fmt.Sprintf("DemoServerFrontpage")
	var positions []models.PlayerPosition
	if err := cache.Get(cacheKey, &positions); err == nil {
		server.MapConfig.TilesURL = rewriteTilesURL(server.MapConfig.TilesURL)
		c.JSON(http.StatusOK, gin.H{
			"map_config": server.MapConfig,
			"positions":  positions,
		})
		return
	}

	var allLatestPositions []models.PlayerPosition
	subQuery := database.DB.Model(&models.PlayerPosition{}).
		Select("MAX(id)").
		Where("server_id = ? AND event_timestamp <= ?", serverID, targetTime).
		Group("player_guid")

	err := database.DB.Preload("Faction").Where("id IN (?)", subQuery).Find(&allLatestPositions).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query demo positions."})
		return
	}

	const activityWindow = 15 * time.Second
	for _, pos := range allLatestPositions {
		windowStart := targetTime.Add(-activityWindow)
		if !pos.EventTimestamp.Before(windowStart) {
			positions = append(positions, pos)
		}
	}

	server.MapConfig.TilesURL = rewriteTilesURL(server.MapConfig.TilesURL)

	// 4. Filtere in Go, um nur die wirklich letzte Position pro Spieler zu behalten.
	// Dies ist eine Vereinfachung, die gut genug für die Demo ist.
	cache.Set(cacheKey, positions, 24*time.Hour)
	c.JSON(http.StatusOK, gin.H{
		"map_config": server.MapConfig,
		"positions":  positions,
	})
}
