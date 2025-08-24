// api/handlers/server_actions.go
package handlers

import (
	"gtglivemap/models"
	"gtglivemap/pkg/logfetcher"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestConnectionHandler versucht, eine Verbindung basierend auf den
// übermittelten, unverschlüsselten Daten aufzubauen.
func TestConnectionHandler(c *gin.Context) {

	var req struct {
		LogSourceType string `json:"log_source_type" binding:"required"`
		FtpHost       string `json:"ftp_host"`
		FtpPort       int    `json:"ftp_port"`
		FtpUser       string `json:"ftp_user"`
		Password      string `json:"password"` // Passwort kommt als Klartext
		SshKey        string `json:"ssh_key"`  // Key kommt als Klartext
		UseFtps       bool   `json:"use_ftps"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request: " + err.Error()})
		return
	}

	// Erstelle ein temporäres Server-Objekt mit den unverschlüsselten Testdaten
	// Wir verschlüsseln die Daten hier NICHT, da wir nur die Verbindung testen.
	testServer := models.Server{
		LogSourceType: req.LogSourceType,
		FtpHost:       req.FtpHost,
		FtpPort:       req.FtpPort,
		FtpUser:       req.FtpUser,
		UseFtps:       req.UseFtps,
		// Wichtig: Wir übergeben die Klartext-Daten für den Test
		// Diese werden als Byte-Slices für die Test-Funktion erwartet.
		EncryptedPassword: []byte(req.Password),
		EncryptedSshKey:   []byte(req.SshKey),
	}

	// Wir rufen eine Test-Funktion auf, die eine echte Verbindung versucht.
	err := logfetcher.TestConnection(testServer)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Connection failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Connection successful!"})
}
