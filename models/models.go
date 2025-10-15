package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MapConfig struct {
	ID              uint   `gorm:"primaryKey"`
	Name            string `gorm:"size:100;not null;unique"`
	TilesURL        string `gorm:"size:255;not null"`
	CrsType         string `gorm:"type:varchar(20);default:'CustomSimple';check:crs_type IN ('CustomSimple', 'Simple')"`
	BoundsSWX       float64
	BoundsSWY       float64
	BoundsNEX       float64
	BoundsNEY       float64
	MinZoom         int
	MaxZoom         int
	InitialViewLat  *float64
	InitialViewLng  *float64
	InitialViewZoom *int
	OffsetX         *float64
	OffsetY         *float64
	ScaleX          *float64 `gorm:"column:scale_x"`
	ScaleY          *float64 `gorm:"column:scale_y"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (MapConfig) TableName() string {
	return "map_configs"
}

type Server struct {
	ID uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`

	OwnerID     uint64 `gorm:"index"`
	MapConfigID uint   `gorm:"not null"`

	Name           string `gorm:"size:100;not null"`
	APIKey         string `gorm:"size:255;unique;not null"`
	IsPublic       bool   `gorm:"default:false"`
	IsListed       bool   `gorm:"default:false;index"`
	MaxStorageDays int    `gorm:"default:7"`

	LogSourceType                  string `gorm:"type:varchar(10);default:'api';check:log_source_type IN ('api', 'ftp', 'sftp')"`
	FtpHost                        string `gorm:"size:255"`
	FtpPort                        int    `gorm:"default:21"`
	FtpUser                        string `gorm:"size:255"`
	EncryptedPassword              []byte `gorm:"type:bytea"`
	EncryptedSshKey                []byte `gorm:"type:bytea"`
	UseFtps                        bool   `gorm:"default:false"`
	ProfileFolderPath              string `gorm:"size:255"`
	LastProcessedDamageTimestamp   *time.Time
	LastProcessedKillTimestamp     *time.Time
	LastProcessedPositionTimestamp *time.Time

	// Zeitstempel
	CreatedAt time.Time
	UpdatedAt time.Time

	// GORM relations (for preloads)
	MapConfig MapConfig `gorm:"foreignKey:MapConfigID"`
}

// TableName gibt den expliziten Namen der Tabelle an.
func (Server) TableName() string {
	return "servers"
}

// BeforeCreate is a GORM hook that is executed before creating a new record.
// We use it to automatically generate a new UUID for the server.
func (s *Server) BeforeCreate(tx *gorm.DB) (err error) {
	// Check if the ID is already set.
	// Wenn nicht (der Standardfall bei Neuanlage), generiere eine neue.
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return
}

type Faction struct {
	ID        uint      `gorm:"primaryKey"`
	ServerID  uuid.UUID `gorm:"type:uuid;index"`
	Name      string    `gorm:"size:100;not null;uniqueIndex:idx_server_faction_name"`
	ColorR    float64
	ColorG    float64
	ColorB    float64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Faction) TableName() string {
	return "factions"
}

type PlayerPosition struct {
	ID             uint      `gorm:"primaryKey;autoIncrement:false"`
	EventTimestamp time.Time `gorm:"primaryKey"`
	PlayerGUID     string    `gorm:"type:text;index:idx_server_player_time"`
	ServerID       uuid.UUID `gorm:"type:uuid;index:idx_server_player_time"`
	FactionID      uint      `gorm:"not null"`
	AbsolutePosX   float64
	AbsolutePosY   float64
	AbsolutePosZ   float64
	RotationX      float64
	RotationY      float64
	RotationZ      float64
	InVehicle      bool
	Faction        Faction `gorm:"foreignKey:FactionID"`
}

func (PlayerPosition) TableName() string {
	return "player_positions"
}

// models/models.go (add at the end)

// PositionPayload is the outer wrapper of the JSON data from the game server.
// Diese PositionData-Struktur wird jetzt vom INNEREN JSON-String verwendet
type PositionData struct {
	Timestamp int64       `json:"timestamp"`
	Position  Coordinates `json:"position"`
	Rotation  Rotation    `json:"rotation"`
	InVehicle int         `json:"inVehicle"` // 0 for false, 1 for true
}

// Dies ist die Struktur des INNEREN JSON-Strings, nachdem er geparst wurde.
// Sie entspricht Ihrer `singlePlayerPayload`
type PositionPayload struct {
	PlayerGUID   string         `json:"playerGuid"`
	PlayerName   string         `json:"playerName"` // We add this for names
	FactionName  string         `json:"factionName"`
	FactionColor FactionColor   `json:"factionColor"`
	Positions    []PositionData `json:"positions"`
}

// These structures remain largely the same
type FactionColor struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
}
type Coordinates struct {
	Absolute Vec3 `json:"absolute"`
	Relative Vec3 `json:"relative"` // Added based on the prototype
}
type Rotation struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type User struct {
	ID          uint64 `gorm:"primaryKey"` // Discord User ID ist der primäre Schlüssel
	Username    string `gorm:"size:100"`
	Avatar      string `gorm:"size:255"`
	AccountType string `gorm:"type:varchar(10);default:'owner';check:account_type IN ('owner', 'admin')"`
	MaxServers  uint   `gorm:"default:10"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (User) TableName() string {
	return "users"
}

type DamageEvent struct {
	ID             uint      `gorm:"primaryKey;autoIncrement:false"`
	EventTimestamp time.Time `gorm:"primaryKey"`
	ServerID       uuid.UUID `gorm:"type:uuid;index:idx_server_event_time"`
	KillerGUID     string    `gorm:"type:text;not null"`
	VictimGUID     string    `gorm:"type:text;not null"`
	WeaponName     string    `gorm:"type:text"`
	DamageAmount   float64
	Distance       float64
	HitZone        string `gorm:"type:text"`
	IsFriendlyFire bool
	IsKill         bool `gorm:"default:false"`
}

func (DamageEvent) TableName() string {
	return "damage_events"
}

type DamagePayload struct {
	Events []DamageEventData `json:"events"`
}

type DamageEventData struct {
	Timestamp      int64   `json:"timestamp"`
	KillerGUID     string  `json:"killerGuid"`
	VictimGUID     string  `json:"victimGuid"`
	WeaponName     string  `json:"weaponName"`
	DamageAmount   float64 `json:"damageAmount"`
	Distance       float64 `json:"distance"`
	HitZone        string  `json:"hitZone"`
	IsFriendlyFire bool    `json:"isFriendlyFire"`
}
type ServerAccess struct {
	UserID    uint64    `gorm:"primaryKey"`
	ServerID  uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time

	User   User   `gorm:"foreignKey:UserID"`
	Server Server `gorm:"foreignKey:ServerID"`
}

func (ServerAccess) TableName() string {
	return "server_access"
}

type PlayerIdentity struct {
	GUID          string    `gorm:"primaryKey;size:255"`
	LastKnownName string    `gorm:"size:100;index"`
	LastSeenAt    time.Time `gorm:"index"`
}

func (PlayerIdentity) TableName() string {
	return "player_identities"
}

type BadWord struct {
	Word         string `gorm:"primaryKey;size:100"` // The word itself is the primary key
	LanguageCode string `gorm:"size:5;index"`        // z.B. 'de', 'en', 'es', 'multi'
}

func (BadWord) TableName() string {
	return "bad_words"
}

// SystemSetting stores general key-value settings for the application.
type SystemSetting struct {
	// The key of the setting, e.g. "demo_server_id"
	Key string `gorm:"column:key;primaryKey;size:100"`
	// The value of the setting, e.g. a server ID or a timestamp
	Value string `gorm:"size:255"`
}

func (SystemSetting) TableName() string {
	return "system_settings"
}
