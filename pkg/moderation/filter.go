// pkg/moderation/filter.go
package moderation

import (
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"strings"
	"sync"
	"time"
)

var (
	badWords      = make(map[string]bool)
	mu            sync.RWMutex
	isInitialized = false
)

func InitBadWordFilter() {
	if isInitialized {
		return
	}
	log.Println("Initializing Bad Word Filter...")
	loadBadWordsFromDB()

	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			log.Println("Refreshing Bad Word list from database...")
			loadBadWordsFromDB()
		}
	}()
	isInitialized = true
}

func loadBadWordsFromDB() {
	var words []models.BadWord
	if err := database.DB.Find(&words).Error; err != nil {
		log.Printf("ERROR: Could not load bad words from database: %v", err)
		return
	}

	newWordMap := make(map[string]bool)
	for _, w := range words {
		newWordMap[strings.ToLower(w.Word)] = true
	}

	mu.Lock()
	badWords = newWordMap
	mu.Unlock()

	log.Printf("Successfully loaded/refreshed %d bad words.", len(badWords))
}

func IsClean(text string) bool {
	lowerText := strings.ToLower(text)
	wordsInText := strings.Fields(lowerText)

	mu.RLock()
	defer mu.RUnlock()

	for _, word := range wordsInText {
		for badWord := range badWords {
			if strings.Contains(word, badWord) {
				log.Printf("Moderation: Found bad word '%s' in text '%s'", badWord, text)
				return false // Nicht sauber!
			}
		}
	}
	return true // Sauber!
}
