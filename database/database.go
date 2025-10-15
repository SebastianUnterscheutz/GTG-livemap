package database

import (
	"fmt"
	"gtglivemap/config"
	"gtglivemap/models"
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

	log.Println("Enabling TimescaleDB extension...")
	if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE").Error; err != nil {
		log.Printf("Warning: Failed to enable TimescaleDB extension: %v", err)
		log.Println("Continuing without TimescaleDB - make sure you're using a TimescaleDB-enabled PostgreSQL image")
	} else {
		log.Println("TimescaleDB extension enabled successfully")
	}

	log.Println("Checking for and removing outdated primary keys...")
	if DB.Migrator().HasConstraint("player_positions", "player_positions_pkey") {
		DB.Exec("ALTER TABLE player_positions DROP CONSTRAINT player_positions_pkey")
		log.Println("Dropped old primary key from 'player_positions'")
	}
	if DB.Migrator().HasConstraint("damage_events", "damage_events_pkey") {
		DB.Exec("ALTER TABLE damage_events DROP CONSTRAINT damage_events_pkey")
		log.Println("Dropped old primary key from 'damage_events'")
	}

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

	log.Println("Converting time-series tables to TimescaleDB hypertables...")
	if err := DB.Exec(`
       SELECT create_hypertable('player_positions', 'event_timestamp',
          chunk_time_interval => INTERVAL '1 day',
          if_not_exists => TRUE
       )
    `).Error; err != nil {
		log.Printf("Warning: Failed to convert player_positions to hypertable: %v", err)
	} else {
		log.Println("player_positions table converted to hypertable successfully")
	}
	if err := DB.Exec(`
       SELECT create_hypertable('damage_events', 'event_timestamp',
          chunk_time_interval => INTERVAL '1 day',
          if_not_exists => TRUE
       )
    `).Error; err != nil {
		log.Printf("Warning: Failed to convert damage_events to hypertable: %v", err)
	} else {
		log.Println("damage_events table converted to hypertable successfully")
	}
}

func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }

func Seed() {
	log.Println("Seeding database: Checking for map configurations...")

	mapsToSeed := []models.MapConfig{
		{Name: "Everon", TilesURL: "https://reforger.recoil.org/map-tiles/everon/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 0, BoundsSWY: 0, BoundsNEX: 12800, BoundsNEY: 12800, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(5254.2203039999995), InitialViewLng: float64Ptr(6868.190718751167), InitialViewZoom: intPtr(1)},
		{Name: "Arland", TilesURL: "https://reforger.recoil.org/map-tiles/arland/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -130, BoundsSWY: -24, BoundsNEX: 20000, BoundsNEY: 20000, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(1347.657804), InitialViewLng: float64Ptr(2718.504380905679), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(4.070010552945661), OffsetY: float64Ptr(-3.950201559959254), ScaleX: float64Ptr(0.9999173445912085), ScaleY: float64Ptr(1.0005111336032386)},
		{Name: "Everon x Arland", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/LODEveronXArland/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 0, BoundsSWY: 0, BoundsNEX: 12800, BoundsNEY: 12800, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(3254.0603039999996), InitialViewLng: float64Ptr(3449.569730781334), InitialViewZoom: intPtr(1)},
		{Name: "Anizay", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/LODAnizay/LODAnizay/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 0, BoundsSWY: 0, BoundsNEX: 10465, BoundsNEY: 10346, MinZoom: -1, MaxZoom: 6, InitialViewLat: float64Ptr(2591.507304), InitialViewLng: float64Ptr(2774.387584094179), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(2.0511680642790324), OffsetY: float64Ptr(-2.1224963035045974), ScaleX: float64Ptr(0.5000158693037552), ScaleY: float64Ptr(0.49975907444053963)},
		{Name: "Rostov", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/LODRostov/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -109, BoundsSWY: -108, BoundsNEX: 8000, BoundsNEY: 8000, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(3447.8258039999996), InitialViewLng: float64Ptr(3849.394724594392), InitialViewZoom: intPtr(1)},
		{Name: "Mussalo", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Mussalo/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -116.996, BoundsSWY: 102.028, BoundsNEX: 11992.318, BoundsNEY: 6764.778, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(3591.5873039999997), InitialViewLng: float64Ptr(4793.458446000001), InitialViewZoom: intPtr(1)},
		{Name: "Ruha", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Ruha/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -105.942, BoundsSWY: -110.053, BoundsNEX: 8498.6764, BoundsNEY: 8404.518, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(5604.248304), InitialViewLng: float64Ptr(4636.535497689279), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(4.116744771375409), OffsetY: float64Ptr(-3.94062970973107), ScaleX: float64Ptr(0.9996781916306572), ScaleY: float64Ptr(1.000103925155197)},
		{Name: "Iraq", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Iraq/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 6.031, BoundsSWY: 49.839, BoundsNEX: 4154.16, BoundsNEY: 4143.03, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(2078.966304), InitialViewLng: float64Ptr(1899.476946), InitialViewZoom: intPtr(1)},
		{Name: "Fallujah", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Fallujah/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -1940.042, BoundsSWY: -1130.889, BoundsNEX: 10539.768, BoundsNEY: 11262.459, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(2266.481304), InitialViewLng: float64Ptr(1811.7317245943914), InitialViewZoom: intPtr(1)},
		{Name: "Gogland", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Gogland/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 0, BoundsSWY: 0, BoundsNEX: 10539.768, BoundsNEY: 11262.459, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(6529.322303999999), InitialViewLng: float64Ptr(5831.041446), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(4.08162648121646), OffsetY: float64Ptr(-3.8740105665231113), ScaleX: float64Ptr(0.9998367573327293), ScaleY: float64Ptr(1.0002582409141911)},
		{Name: "BelleauWood", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/BelleauWood/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -409.6764, BoundsSWY: -1033.513, BoundsNEX: 12137.782, BoundsNEY: 12135.137, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(5597.997804), InitialViewLng: float64Ptr(7474.922946000001), InitialViewZoom: intPtr(1)},
		{Name: "Tarkistan", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Tarkistan/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -6551.201, BoundsSWY: -6551.201, BoundsNEX: 19553.602, BoundsNEY: 19553.602, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(9079.526303999999), InitialViewLng: float64Ptr(4336.179630627672), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(4.133062814639231), OffsetY: float64Ptr(-4.172139572032279), ScaleX: float64Ptr(0.9998108259059848), ScaleY: float64Ptr(0.999853503880906)},
		{Name: "Killhouse", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Killhouse/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 0, BoundsSWY: 0, BoundsNEX: 10000, BoundsNEY: 10000, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(0), InitialViewLng: float64Ptr(0), InitialViewZoom: intPtr(1)},
		{Name: "Zarichne", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Zarichne/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -3223.852, BoundsSWY: -8022.527, BoundsNEX: 25921.398, BoundsNEY: 27314.527, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(1364.846679), InitialViewLng: float64Ptr(2776.8908835), InitialViewZoom: intPtr(3)},
		{Name: "Kunar", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Kunar/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -3428.007, BoundsSWY: -3837.543, BoundsNEX: 10542, BoundsNEY: 10662, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(2770.625636123613), InitialViewLng: float64Ptr(3053.8990309536175), InitialViewZoom: intPtr(4), OffsetX: float64Ptr(4.075104960269101), OffsetY: float64Ptr(-4.110000645643765), ScaleX: float64Ptr(0.9996272736131538), ScaleY: float64Ptr(0.9994895314703269)},
		{Name: "Zimnitrita", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Zimnitrita/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -3223.852, BoundsSWY: -8022.527, BoundsNEX: 25921.398, BoundsNEY: 27314.527, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(12617.309304), InitialViewLng: float64Ptr(9194.040789542707), InitialViewZoom: intPtr(1)},
		{Name: "WorthyIslands", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/WorthyIslands/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -100, BoundsSWY: -100, BoundsNEX: 3266.865, BoundsNEY: 3350.562, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(1247.6498040000001), InitialViewLng: float64Ptr(2054.176821), InitialViewZoom: intPtr(2), OffsetX: float64Ptr(4.04190836818492), OffsetY: float64Ptr(-4.060294183512042), ScaleX: float64Ptr(0.9996916293667232), ScaleY: float64Ptr(0.9995325917838532)},
		{Name: "Pantelleria", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/LODPantelleria/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -100, BoundsSWY: -100, BoundsNEX: 14775.546, BoundsNEY: 15027.955, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(5716.757304), InitialViewLng: float64Ptr(10513.0913137422), InitialViewZoom: intPtr(1)},
		{Name: "Novka", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Novka/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: 0, BoundsSWY: 0, BoundsNEX: 20000, BoundsNEY: 20000, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(817.9279290000001), InitialViewLng: float64Ptr(2157.310071), InitialViewZoom: intPtr(3)},
		{Name: "WCS_Serhiivka", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Tiles_WCS_Serhiivka/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -107, BoundsSWY: -5, BoundsNEX: 10347, BoundsNEY: 10346, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(5621.705701606561), InitialViewLng: float64Ptr(5308.629332753242), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(4.071338769626692), OffsetY: float64Ptr(-4.198271491889102), ScaleX: float64Ptr(0.9999514813510093), ScaleY: float64Ptr(0.999608380696849)},
		{Name: "Nizla", TilesURL: "https://gtg-cdn.fsn1.your-objectstorage.com/Tiles_Nizla/{z}/{x}/{y}/tile.jpg", CrsType: "CustomSimple", BoundsSWX: -4331, BoundsSWY: -3968, BoundsNEX: 11581, BoundsNEY: 12476, MinZoom: 0, MaxZoom: 5, InitialViewLat: float64Ptr(4209.432328657315), InitialViewLng: float64Ptr(3314.7652632783806), InitialViewZoom: intPtr(1), OffsetX: float64Ptr(4.087745073735846), OffsetY: float64Ptr(-4.025400944057878), ScaleX: float64Ptr(0.99980579808386), ScaleY: float64Ptr(0.9999973820344734)},
	}

	for _, mapData := range mapsToSeed {
		result := DB.Where(models.MapConfig{Name: mapData.Name}).
			Attrs(mapData).
			FirstOrCreate(&models.MapConfig{})
		if result.Error != nil {
			log.Printf("Error seeding map %s: %v", mapData.Name, result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("Successfully seeded map: %s", mapData.Name)
		}
	}
	log.Println("Map seeding process completed.")

	var serverCount int64
	DB.Model(&models.Server{}).Count(&serverCount)
	if serverCount == 0 {
		log.Println("Seeding database: Creating default server...")
		hashedAPIKey := "secret-my-test-api-key-123"

		var defaultMap models.MapConfig
		DB.Where("name = ?", "Everon").First(&defaultMap)

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
			log.Println("Could not seed server because no default map config ('Everon') was found.")
		}
	}
}
