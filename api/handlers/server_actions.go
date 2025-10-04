// api/handlers/server_actions.go
package handlers

import (
	"gtglivemap/models"
	"gtglivemap/pkg/logfetcher"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestConnectionHandler attempts to establish a connection based on the
// submitted, unencrypted data.
func TestConnectionHandler(c *gin.Context) {

	var req struct {
		LogSourceType string `json:"log_source_type" binding:"required"`
		FtpHost       string `json:"ftp_host"`
		FtpPort       int    `json:"ftp_port"`
		FtpUser       string `json:"ftp_user"`
		Password      string `json:"password"` // Password comes as plaintext
		SshKey        string `json:"ssh_key"`  // Key comes as plaintext
		UseFtps       bool   `json:"use_ftps"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request: " + err.Error()})
		return
	}

	// Create a temporary server object with the unencrypted test data
	// We do NOT encrypt the data here, as we are only testing the connection.
	testServer := models.Server{
		LogSourceType: req.LogSourceType,
		FtpHost:       req.FtpHost,
		FtpPort:       req.FtpPort,
		FtpUser:       req.FtpUser,
		UseFtps:       req.UseFtps,
		// Important: We pass the plaintext data for the test
		// These are expected as byte slices for the test function.
		EncryptedPassword: []byte(req.Password),
		EncryptedSshKey:   []byte(req.SshKey),
	}

	// We call a test function that attempts a real connection.
	err := logfetcher.TestConnection(testServer)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Connection failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Connection successful!"})
}
