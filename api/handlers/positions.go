package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gtglivemap/cache"
	"gtglivemap/database"
	"gtglivemap/models"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

var playerNameRegex = regexp.MustCompile(`"playerName"\s*:\s*"(.*?)"`)

// updateLatestPositionsCache akzeptiert die gerade gespeicherten Positionen direkt, um DB-Lesevorgänge zu vermeiden.
func updateLatestPositionsCache(serverID uuid.UUID, justSavedPositions []models.PlayerPosition) {
	// Definiere die Response-Strukturen, die im Cache gespeichert werden.
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

	// 1. Filtere die übergebenen Positionen nach dem Aktivitätsfenster.
	nowUTC := time.Now().UTC()
	const activityWindow = 15 * time.Second
	windowStart := nowUTC.Add(-activityWindow)

	var activePositions []models.PlayerPosition
	factionIDsToLoad := make(map[uint]bool)

	for _, pos := range justSavedPositions {
		if !pos.EventTimestamp.Before(windowStart) {
			activePositions = append(activePositions, pos)
			factionIDsToLoad[pos.FactionID] = true // Sammle die Faction IDs, die wir nachladen müssen
		}
	}

	// Wenn keine Positionen im Aktivitätsfenster sind, setzen wir einen leeren Cache.
	if len(activePositions) == 0 {
		cacheKey := fmt.Sprintf("latest_positions:%s", serverID.String())
		if err := cache.Set(cacheKey, []PositionResponse{}, 1*time.Minute); err != nil {
			log.Printf("WARN (Cache Update): Could not set empty latest_positions cache for server %s: %v", serverID, err)
		}
		return
	}

	// 2. Lade die benötigten Fraktionsdaten für die aktiven Positionen in einer einzigen, effizienten Abfrage.
	var factionIDs []uint
	for id := range factionIDsToLoad {
		factionIDs = append(factionIDs, id)
	}
	factions := make(map[uint]models.Faction)
	if len(factionIDs) > 0 {
		var factionList []models.Faction
		database.DB.Where("id IN ?", factionIDs).Find(&factionList)
		for _, f := range factionList {
			factions[f.ID] = f
		}
	}

	// 3. Baue die finale Response zusammen, die im Cache gespeichert wird.
	response := make([]PositionResponse, len(activePositions))
	for i, pos := range activePositions {
		faction := factions[pos.FactionID]
		response[i] = PositionResponse{
			PlayerGUID:     pos.PlayerGUID,
			EventTimestamp: pos.EventTimestamp.Unix(),
			AbsolutePosX:   pos.AbsolutePosX,
			AbsolutePosZ:   pos.AbsolutePosZ,
			RotationX:      pos.RotationX,
			InVehicle:      pos.InVehicle,
			Faction: FactionResponse{
				Name:   faction.Name,
				ColorR: faction.ColorR,
				ColorG: faction.ColorG,
				ColorB: faction.ColorB,
			},
		}
	}

	// 4. Speichere das Ergebnis im Cache.
	cacheKey := fmt.Sprintf("latest_positions:%s", serverID.String())
	if err := cache.Set(cacheKey, response, 1*time.Minute); err != nil {
		log.Printf("WARN (Cache Update): Could not set latest_positions cache for server %s: %v", serverID, err)
	}
}

func PostPositionsHandler(c *gin.Context) {
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request.Body)
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	serverID_iface, exists := c.Get("server_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error: server_id context is missing"})
		return
	}
	serverID, ok := serverID_iface.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error: server_id context has wrong type"})
		return
	}

	var playersData map[string]string
	if err := c.ShouldBindJSON(&playersData); err != nil {
		log.Printf("ERROR (ServerID %s): Failed to bind outer JSON (map): %v", serverID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid outer JSON structure."})
		return
	}

	factionCache := make(map[string]models.Faction)
	var allPositionsToSave []models.PlayerPosition
	identitiesToUpdate := make(map[string]string)
	var latestTimestamp time.Time

	playerNameRegex := regexp.MustCompile(`"playerName"\s*:\s*"(.*?)"`)
	for _, jsonString := range playersData {

		matches := playerNameRegex.FindStringSubmatch(jsonString)
		if len(matches) > 1 {
			extractedName := matches[1]
			cleanedName := strings.ReplaceAll(extractedName, `"`, `\"`)

			correctPlayerNameField := fmt.Sprintf(`"playerName": "%s"`, cleanedName)
			jsonString = strings.Replace(jsonString, matches[0], correctPlayerNameField, 1)
		}

		var singlePlayerPayload models.PositionPayload
		if err := json.Unmarshal([]byte(jsonString), &singlePlayerPayload); err != nil {
			log.Printf("WARNING (ServerID %s): Could not parse inner JSON: %v. Skipping player. JSON was: %s", serverID, err, jsonString)
			continue
		}

		if singlePlayerPayload.FactionName == "" {
			log.Printf("WARNING (ServerID %s): FactionName for GUID %s is empty. Skipping player.", serverID, singlePlayerPayload.PlayerGUID)
			continue
		}

		if singlePlayerPayload.PlayerName != "" {
			identitiesToUpdate[singlePlayerPayload.PlayerGUID] = singlePlayerPayload.PlayerName
		}

		faction, found := factionCache[singlePlayerPayload.FactionName]
		if !found {
			factionToHandle := models.Faction{ServerID: serverID, Name: singlePlayerPayload.FactionName, ColorR: singlePlayerPayload.FactionColor.R, ColorG: singlePlayerPayload.FactionColor.G, ColorB: singlePlayerPayload.FactionColor.B}
			result := database.DB.Where(models.Faction{ServerID: serverID, Name: singlePlayerPayload.FactionName}).FirstOrCreate(&faction, factionToHandle)
			if result.Error != nil {
				log.Printf("ERROR (ServerID %s): Could not find or create faction '%s': %v", serverID, singlePlayerPayload.FactionName, result.Error)
				continue
			}
			factionCache[faction.Name] = faction
		}

		for _, posData := range singlePlayerPayload.Positions {
			eventTime := time.Unix(posData.Timestamp, 0)
			if eventTime.After(latestTimestamp) {
				latestTimestamp = eventTime
			}

			dbRecord := models.PlayerPosition{
				PlayerGUID:     singlePlayerPayload.PlayerGUID,
				ServerID:       serverID,
				FactionID:      faction.ID,
				EventTimestamp: time.Unix(posData.Timestamp, 0),
				AbsolutePosX:   posData.Position.Absolute.X,
				AbsolutePosY:   posData.Position.Absolute.Y,
				AbsolutePosZ:   posData.Position.Absolute.Z,
				RotationX:      posData.Rotation.X,
				RotationY:      posData.Rotation.Y,
				RotationZ:      posData.Rotation.Z,
				InVehicle:      posData.InVehicle == 1,
			}
			allPositionsToSave = append(allPositionsToSave, dbRecord)
		}
	}

	go processNameUpdates(identitiesToUpdate)

	if len(allPositionsToSave) > 0 {
		if err := database.DB.Create(&allPositionsToSave).Error; err != nil {
			log.Printf("ERROR (ServerID %s): Could not save positions batch to DB: %v", serverID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save position data."})
			return
		}

		if !latestTimestamp.IsZero() {
			go func(sID uuid.UUID, ts time.Time) {
				if err := database.DB.Model(&models.Server{}).Where("id = ?", sID).Update("last_processed_position_timestamp", ts).Error; err != nil {
					log.Printf("ERROR (Async Timestamp Update): Could not update last_processed_position_timestamp for server %s: %v", sID, err)
				}
			}(serverID, latestTimestamp)
		}

		go updateLatestPositionsCache(serverID, allPositionsToSave)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Data received successfully",
		"records_saved": len(allPositionsToSave),
	})
}

// ★★★ NEUE ASYNCHRONE FUNKTION FÜR NAMEN-UPDATES ★★★
func processNameUpdates(identitiesToUpdate map[string]string) {
	if len(identitiesToUpdate) == 0 {
		return
	}

	// Wandle die Map in eine Slice von PlayerIdentity-Structs um, die GORM versteht.
	var identitySlice []models.PlayerIdentity
	for guid, name := range identitiesToUpdate {
		identitySlice = append(identitySlice, models.PlayerIdentity{
			GUID:          guid,
			LastKnownName: name,
			LastSeenAt:    time.Now().UTC(),
		})
	}

	if err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "guid"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_known_name", "last_seen_at"}),
	}).Create(&identitySlice).Error; err != nil {
		log.Printf("ERROR (Async Name Update): Could not update player names: %v", err)
	}
}
