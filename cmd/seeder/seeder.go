// cmd/seeder/seeder.go
package seeder

import (
	"bufio"
	"gtglivemap/database"
	"gtglivemap/models"
	"log"
	"net/http"
	"strings"

	"gorm.io/gorm/clause"
)

var LanguageMap = map[string]string{
	"de": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/de",
	"en": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/en",
	"es": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/es",
	"fr": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/fr",
	"it": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/it",
	"pt": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/pt",
	"nl": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/nl",

	"da": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/da",
	"fi": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/fi",
	"no": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/no",
	"sv": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/sv",

	"cs": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/cs",
	"hu": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/hu",
	"pl": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/pl",
	"ru": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/ru",

	"zh":  "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/zh",
	"hi":  "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/hi",
	"ja":  "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/ja",
	"ko":  "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/ko",
	"th":  "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/th",
	"fil": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/fil", // Filipino

	"ar": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/ar",
	"fa": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/fa", // Persian
	"tr": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/tr",

	"eo":  "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/eo",  // Esperanto
	"tlh": "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/tlh", // Klingon
}

// SeedBadWords fetches and populates the database with inappropriate words for content moderation.
func SeedBadWords() {
	log.Println("Starting to seed bad words from public repository...")
	totalWordsAdded := 0

	for lang, url := range LanguageMap {
		log.Printf("Fetching bad words for language: %s", lang)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("ERROR: Failed to download bad words list for %s: %v", lang, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("ERROR: Received non-200 status code for %s list: %d", lang, resp.StatusCode)
			continue
		}

		var wordsToInsert []models.BadWord
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			word := strings.TrimSpace(scanner.Text())
			if word != "" {
				wordsToInsert = append(wordsToInsert, models.BadWord{
					Word:         word,
					LanguageCode: lang,
				})
			}
		}

		if len(wordsToInsert) > 0 {
			result := database.DB.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(&wordsToInsert)

			if result.Error != nil {
				log.Printf("ERROR: Failed to insert bad words for %s into database: %v", lang, result.Error)
			} else {
				log.Printf("Successfully added/updated %d words for language %s.", result.RowsAffected, lang)
				totalWordsAdded += int(result.RowsAffected)
			}
		}
	}
	log.Printf("Bad words seeding finished. Total new words added: %d", totalWordsAdded)
}
