package logfetcher

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"gtglivemap/database"
	"gtglivemap/models"
	"gtglivemap/utils"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type LogParserFunc func(line string, serverID uuid.UUID) (*models.DamageEvent, bool)

func ProcessServerLogs(server models.Server) {
	log.Printf("LogFetcher: Processing server ID %s (%s)", server.ID, server.Name)

	damageLogPath := fmt.Sprintf("%s/profile/RJS/events/damageEvents.log", server.ProfileFolderPath)
	lastDamageTimestamp := time.Time{}
	if server.LastProcessedDamageTimestamp != nil {
		lastDamageTimestamp = *server.LastProcessedDamageTimestamp
	}
	newDamageEvents, highestDamageTs := fetchAndParseLogs(server, damageLogPath, lastDamageTimestamp, parseDamageLogLine)
	if len(newDamageEvents) > 0 {
		SaveEventsInBatches(server, newDamageEvents, highestDamageTs, "damage")
	}

	killLogPath := fmt.Sprintf("%s/profile/RJS/events/playerKilledEvents.log", server.ProfileFolderPath)
	lastKillTimestamp := time.Time{}
	if server.LastProcessedKillTimestamp != nil {
		lastKillTimestamp = *server.LastProcessedKillTimestamp
	}
	newKillEvents, highestKillTs := fetchAndParseLogs(server, killLogPath, lastKillTimestamp, parseKillLogLine)
	if len(newKillEvents) > 0 {
		SaveEventsInBatches(server, newKillEvents, highestKillTs, "kill")
	}
}

func SaveEventsInBatches(server models.Server, events []models.DamageEvent, newTimestamp time.Time, eventType string) {
	const batchSize = 200
	totalEvents := len(events)
	var batchesCreated int
	for i := 0; i < totalEvents; i += batchSize {
		end := i + batchSize
		if end > totalEvents {
			end = totalEvents
		}
		batch := events[i:end]
		if err := database.DB.Create(&batch).Error; err != nil {
			log.Printf("ERROR writing %s event BATCH for server %s to DB: %v", eventType, server.ID, err)
			return
		}
		batchesCreated++
	}
	log.Printf("SUCCESS: Wrote %d new %s events for server %s in %d batches.", totalEvents, eventType, server.ID, batchesCreated)
	if eventType == "damage" {
		database.DB.Model(&server).Update("last_processed_damage_timestamp", newTimestamp)
	} else if eventType == "kill" {
		database.DB.Model(&server).Update("last_processed_kill_timestamp", newTimestamp)
	}
}

func fetchAndParseLogs(server models.Server, path string, lastTimestamp time.Time, parserFunc LogParserFunc) ([]models.DamageEvent, time.Time) {
	var logContent io.Reader
	var err error

	if server.LogSourceType == "sftp" {
		logContent, err = downloadSftpFile(server, path)
	} else if server.LogSourceType == "ftp" {
		logContent, err = downloadFtpFile(server, path)
	}
	if err != nil {
		if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "Failed to retrieve") {
			log.Printf("Info: Log file not found for server %s at path %s. Skipping.", server.ID, path)
		} else {
			log.Printf("ERROR downloading log for server %s: %v", server.ID, err)
		}
		return nil, lastTimestamp
	}

	scanner := bufio.NewScanner(logContent)
	var newEvents []models.DamageEvent
	highestTimestampInFile := lastTimestamp
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		event, ok := parserFunc(line, server.ID)
		if !ok {
			continue
		}
		if event.EventTimestamp.After(lastTimestamp) {
			newEvents = append(newEvents, *event)
			if event.EventTimestamp.After(highestTimestampInFile) {
				highestTimestampInFile = event.EventTimestamp
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading log content for server %s: %v", server.ID, err)
	}
	log.Printf("LogFetcher: Scanned %d lines from %s for server %s. Found %d new events.", lineCount, path, server.ID, len(newEvents))
	return newEvents, highestTimestampInFile
}

func extractValue(line, key string) string {
	re := regexp.MustCompile(key + ` = ([^,]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func parseDamageLogLine(line string, serverID uuid.UUID) (*models.DamageEvent, bool) {
	tsRe := regexp.MustCompile(`\[(.*?)\]`)
	tsMatch := tsRe.FindStringSubmatch(line)
	if len(tsMatch) < 2 {
		return nil, false
	}
	ts, err := time.Parse("2006-01-02 15:04:05", tsMatch[1])
	if err != nil {
		return nil, false
	}

	victimBiId := extractValue(line, "victimBiId")
	killerBiId := extractValue(line, "killerBiId")
	weaponName := extractValue(line, "weaponName")
	damageAmountStr := extractValue(line, "damageAmount")
	distanceStr := extractValue(line, "distance")
	hitZone := extractValue(line, "hitZoneName")
	isFriendlyFireStr := extractValue(line, "isFriendlyFire")

	damage, _ := strconv.ParseFloat(damageAmountStr, 64)
	distance, _ := strconv.ParseFloat(distanceStr, 64)
	isFriendly, _ := strconv.ParseBool(isFriendlyFireStr)

	if victimBiId == "" || killerBiId == "" {
		return nil, false
	}

	return &models.DamageEvent{ServerID: serverID, EventTimestamp: ts, VictimGUID: victimBiId, KillerGUID: killerBiId, WeaponName: weaponName, DamageAmount: damage, Distance: distance, HitZone: hitZone, IsFriendlyFire: isFriendly, IsKill: false}, true
}

func parseKillLogLine(line string, serverID uuid.UUID) (*models.DamageEvent, bool) {
	tsRe := regexp.MustCompile(`\[(.*?)\]`)
	tsMatch := tsRe.FindStringSubmatch(line)
	if len(tsMatch) < 2 {
		return nil, false
	}
	ts, err := time.Parse("2006-01-02 15:04:05", tsMatch[1])
	if err != nil {
		return nil, false
	}

	victimBiId := extractValue(line, "victimBiId")
	killerBiId := extractValue(line, "killerBiId")
	weaponName := extractValue(line, "weaponName")
	distanceStr := extractValue(line, "killDistance")
	isFriendlyFireStr := extractValue(line, "isFriendlyFire")

	distance, _ := strconv.ParseFloat(distanceStr, 64)
	isFriendly, _ := strconv.ParseBool(isFriendlyFireStr)

	if victimBiId == "" || killerBiId == "" {
		return nil, false
	}

	return &models.DamageEvent{ServerID: serverID, EventTimestamp: ts, VictimGUID: victimBiId, KillerGUID: killerBiId, WeaponName: weaponName, DamageAmount: 100.0, Distance: distance, HitZone: "FATAL", IsFriendlyFire: isFriendly, IsKill: true}, true
}

func TestConnection(server models.Server) error {
	if server.LogSourceType == "sftp" {
		config, err := createSftpConfig(server, true)
		if err != nil {
			return err
		}
		addr := fmt.Sprintf("%s:%d", server.FtpHost, server.FtpPort)
		conn, err := ssh.Dial("tcp", addr, config)
		if err != nil {
			return fmt.Errorf("ssh dial failed: %w", err)
		}
		conn.Close()
		return nil
	} else if server.LogSourceType == "ftp" {
		addr := fmt.Sprintf("%s:%d", server.FtpHost, server.FtpPort)
		var conn *ftp.ServerConn
		var err error
		if server.UseFtps {
			conn, err = ftp.Dial(addr, ftp.DialWithExplicitTLS(&tls.Config{InsecureSkipVerify: true}))
		} else {
			conn, err = ftp.Dial(addr, ftp.DialWithTimeout(10*time.Second))
		}
		if err != nil {
			return fmt.Errorf("ftp dial failed: %w", err)
		}
		err = conn.Login(server.FtpUser, string(server.EncryptedPassword))
		if err != nil {
			return fmt.Errorf("ftp login failed: %w", err)
		}
		conn.Quit()
		return nil
	}
	return fmt.Errorf("unsupported log source type for testing")
}

func createSftpConfig(server models.Server, isTest bool) (*ssh.ClientConfig, error) {
	var auth ssh.AuthMethod
	var passBytes, keyBytes []byte
	var err error
	if isTest {
		passBytes = server.EncryptedPassword
		keyBytes = server.EncryptedSshKey
	} else {
		if len(server.EncryptedPassword) > 0 {
			passBytes, err = utils.Decrypt(server.EncryptedPassword)
			if err != nil {
				return nil, err
			}
		}
		if len(server.EncryptedSshKey) > 0 {
			keyBytes, err = utils.Decrypt(server.EncryptedSshKey)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(keyBytes) > 0 {
		var signer ssh.Signer
		if len(passBytes) > 0 {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, passBytes)
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("could not parse ssh private key: %w", err)
		}
		auth = ssh.PublicKeys(signer)
	} else if len(passBytes) > 0 {
		auth = ssh.Password(string(passBytes))
	} else {
		return nil, fmt.Errorf("no credentials configured")
	}
	return &ssh.ClientConfig{User: server.FtpUser, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second}, nil
}

func downloadSftpFile(server models.Server, path string) (io.Reader, error) {
	config, err := createSftpConfig(server, false)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", server.FtpHost, server.FtpPort)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial ssh: %w", err)
	}
	defer conn.Close()
	client, err := sftp.NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to create sftp client: %w", err)
	}
	defer client.Close()
	file, err := client.Open(ToSlash(path))
	if err != nil {
		return nil, fmt.Errorf("failed to open sftp file: %w", err)
	}
	defer file.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		return nil, fmt.Errorf("failed to read sftp content: %w", err)
	}
	return &buf, nil
}

func downloadFtpFile(server models.Server, path string) (io.Reader, error) {
	addr := fmt.Sprintf("%s:%d", server.FtpHost, server.FtpPort)
	var conn *ftp.ServerConn
	var err error
	if server.UseFtps {
		conn, err = ftp.Dial(addr, ftp.DialWithExplicitTLS(&tls.Config{InsecureSkipVerify: true}))
	} else {
		conn, err = ftp.Dial(addr, ftp.DialWithTimeout(10*time.Second))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dial ftp: %w", err)
	}
	defer conn.Quit()
	decryptedPass, err := utils.Decrypt(server.EncryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt ftp pass: %w", err)
	}
	if err = conn.Login(server.FtpUser, string(decryptedPass)); err != nil {
		return nil, fmt.Errorf("ftp login failed: %w", err)
	}
	r, err := conn.Retr(ToSlash(path))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ftp file: %w", err)
	}
	defer r.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, fmt.Errorf("failed to read ftp content: %w", err)
	}
	return &buf, nil
}

// ToSlash replaces all backslashes (\) in a string with forward slashes (/).
func ToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
