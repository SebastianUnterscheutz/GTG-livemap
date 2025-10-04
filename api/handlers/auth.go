// api/handlers/auth.go

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"gtglivemap/config"
	"gtglivemap/database"
	"gtglivemap/models"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/boj/redistore"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

var (
	oauthConf   *oauth2.Config
	CookieStore sessions.Store
)

func InitAuth() {
	cfg := config.AppConfig

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	_, err := client.Ping(context.Background()).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}

	oauthConf = &oauth2.Config{
		ClientID:     cfg.Discord.ClientID,
		ClientSecret: cfg.Discord.ClientSecret,
		RedirectURL:  cfg.Discord.RedirectURI,
		Scopes:       []string{"identify"}, // "identify" is sufficient for ID, name and avatar
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/api/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
	}

	CookieStore, err = redistore.NewRediStore(10, "tcp", cfg.Redis.Addr, cfg.Redis.Username, cfg.Redis.Password, ([]byte(cfg.Session.Secret)))
	if err != nil {
		log.Fatalf("Could not create RediStore: %v", err)
	}

}

func HandleDiscordLogin(c *gin.Context) {
	url := oauthConf.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleDiscordCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No code provided"})
		return
	}

	token, err := oauthConf.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token: " + err.Error()})
		return
	}

	client := oauthConf.Client(context.Background(), token)
	resp, err := client.Get("https://discord.com/api/users/@me")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var discordUser struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	}
	if err := json.Unmarshal(body, &discordUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse user info"})
		return
	}

	userID, _ := strconv.ParseUint(discordUser.ID, 10, 64)

	user := models.User{
		ID:       userID,
		Username: discordUser.Username,
		Avatar:   fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", discordUser.ID, discordUser.Avatar),
	}
	if err := database.DB.Where(models.User{ID: userID}).Assign(user).FirstOrCreate(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error during user upsert"})
		return
	}

	log.Printf("User %s (ID: %s) logged in or registered.", user.Username, user.ID)

	session, _ := CookieStore.Get(c.Request, "gtg-livemap-session")
	session.Values["user_id"] = user.ID
	session.Options.MaxAge = 86400 * 7
	session.Options.HttpOnly = true
	err = session.Save(c.Request, c.Writer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session"})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func HandleLogout(c *gin.Context) {
	session, _ := CookieStore.Get(c.Request, "gtg-livemap-session")
	session.Values["user_id"] = 0
	session.Options.MaxAge = -1
	err := session.Save(c.Request, c.Writer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session on logout"})
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, "/")
}
