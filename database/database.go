package database

import (
	"fmt"
	"gtglivemap/config" // <--- ANGEPASST
	"gtglivemap/models" // <--- ANGEPASST
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	cfg := config.AppConfig.Database
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Database connection established")
}

func Migrate() {
	log.Println("Running database migrations...")
	err := DB.AutoMigrate(
		&models.MapConfig{},
		&models.Server{},
		&models.Faction{},
		&models.PlayerPosition{},
		&models.User{},
		&models.DamageEvent{},
		&models.ServerAccess{},
		&models.PlayerIdentity{},
		&models.BadWord{},
		&models.SystemSetting{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database migration completed")
}

func Seed() {
	// Erstellen einer Beispiel-Kartenkonfiguration, falls noch keine existiert
	var mapCount int64
	DB.Model(&models.MapConfig{}).Count(&mapCount)
	if mapCount == 0 {
		log.Println("Seeding database: Creating default map configuration...")
		defaultMap := models.MapConfig{
			Name:      "Everon",
			TilesURL:  "https://reforger.recoil.org/map-tiles/everon/{z}/{x}/{y}/tile.jpg",
			CrsType:   "CustomSimple",
			BoundsSWX: 0,
			BoundsSWY: 0,
			BoundsNEX: 12800,
			BoundsNEY: 12800,
			MinZoom:   0,
			MaxZoom:   5,
		}
		DB.Create(&defaultMap)
	}

	var serverCount int64
	DB.Model(&models.Server{}).Count(&serverCount)
	if serverCount == 0 {
		log.Println("Seeding database: Creating default server...")
		hashedAPIKey := "secret-my-test-api-key-123"

		var defaultMap models.MapConfig
		DB.First(&defaultMap)

		if defaultMap.ID != 0 {
			defaultServer := models.Server{
				Name:        "Mein Test Server",
				MapConfigID: defaultMap.ID,
				APIKey:      hashedAPIKey,
				IsPublic:    true,
			}
			if err := DB.Create(&defaultServer).Error; err != nil {
				log.Printf("Could not create default server: %v", err)
			} else {
				log.Printf("Default server created. Your testing API-KEY is: my-test-api-key-123")
				log.Printf("!!! IMPORTANT: Use this key with the 'X-API-KEY' header for your tests !!!")
			}
		} else {
			log.Println("Could not seed server because no map config was found.")
		}
	}
}
