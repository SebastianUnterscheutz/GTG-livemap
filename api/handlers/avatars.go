// api/handlers/avatars.go
package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
)

var validAvatarHash = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func AvatarProxyHandler(c *gin.Context) {
	userID := c.Param("user_id")
	avatarHash := c.Param("avatar_hash")

	if _, err := strconv.ParseUint(userID, 10, 64); err != nil {
		c.String(http.StatusBadRequest, "Invalid user ID format")
		return
	}
	if !validAvatarHash.MatchString(avatarHash) {
		c.String(http.StatusBadRequest, "Invalid avatar hash format")
		return
	}

	discordURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", userID, avatarHash)

	resp, err := http.Get(discordURL)
	if err != nil {
		log.Printf("Avatar Proxy: Failed to fetch image from Discord: %v", err)
		c.String(http.StatusBadGateway, "Failed to fetch avatar")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.String(resp.StatusCode, "Avatar not found on Discord CDN")
		return
	}

	c.Header("Cache-Control", "public, max-age=86400")

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}

	io.Copy(c.Writer, resp.Body)
}
