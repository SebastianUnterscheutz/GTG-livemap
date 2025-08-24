// api/handlers/proxy.go

package handlers

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Die einzige Domain, zu der unser Proxy eine Verbindung herstellen darf.
const allowedProxyHost = "gtg-cdn.fsn1.your-objectstorage.com"

// CDNProxyHandler ist ein Reverse Proxy, der Anfragen sicher an das CDN weiterleitet.
func CDNProxyHandler(c *gin.Context) {
	// 1. Extrahiere den angefragten Pfad (z.B. "/map/bootstrap.min.css")
	path := c.Param("path")

	// 2. Sicherheitsüberprüfung: Verhindere Path Traversal Angriffe
	if strings.Contains(path, "..") {
		c.String(http.StatusBadRequest, "Invalid path")
		return
	}

	// 3. Baue die vollständige Ziel-URL zusammen
	targetURL := "https://" + allowedProxyHost + path

	// 4. Erstelle eine neue Anfrage an das CDN
	proxyReq, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		log.Printf("Proxy Error: Failed to create request for %s: %v", targetURL, err)
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	// 5. Kopiere den "Origin"-Header, falls vorhanden (gute Praxis für einige CDNs)
	proxyReq.Header.Set("Origin", c.Request.Host)

	// 6. Führe die Anfrage aus
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Proxy Error: Failed to fetch from CDN %s: %v", targetURL, err)
		c.String(http.StatusBadGateway, "Could not reach CDN")
		return
	}
	defer resp.Body.Close()

	// 7. Kopiere die Header vom CDN zur Antwort an den Benutzer.
	// Das ist entscheidend für den korrekten Content-Type und Caching!
	c.Writer.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	c.Writer.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		c.Writer.Header().Set("Cache-Control", cacheControl)
	}

	// 8. Schreibe den Statuscode vom CDN in die Antwort.
	c.Writer.WriteHeader(resp.StatusCode)

	// 9. Streame den Body (die eigentliche Datei) vom CDN direkt zum Benutzer.
	io.Copy(c.Writer, resp.Body)
}

var allowedProxyHosts = map[string]bool{
	"gtg-cdn.fsn1.your-objectstorage.com": true,
	"reforger.recoil.org":                 true,
}

// TilesProxyHandler ist ein intelligenter Reverse Proxy für mehrere Tile-Quellen.
func TilesProxyHandler(c *gin.Context) {
	// 1. Extrahiere den gesamten Pfad, der jetzt "hostname/rest/des/pfades" enthält
	path := c.Param("path")
	if path == "" {
		c.String(http.StatusBadRequest, "Invalid path")
		return
	}

	// Entferne das führende "/", falls vorhanden
	path = strings.TrimPrefix(path, "/")

	// 2. Teile den Pfad in Hostname und den eigentlichen Kachel-Pfad
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		c.String(http.StatusBadRequest, "Invalid proxy path format")
		return
	}
	targetHost := parts[0]
	targetPath := parts[1]

	// 3. ★★★ WICHTIGE SICHERHEITSPRÜFUNG ★★★
	// Prüfe, ob die angeforderte Zieldomain in unserer Whitelist ist.
	if !allowedProxyHosts[targetHost] {
		log.Printf("Proxy Warning: Denied request to untrusted host: %s", targetHost)
		c.String(http.StatusForbidden, "Proxying to this host is not allowed")
		return
	}

	// 4. Sicherheitsüberprüfung auf Path Traversal
	if strings.Contains(targetPath, "..") {
		c.String(http.StatusBadRequest, "Invalid path")
		return
	}

	// 5. Baue die dynamische Ziel-URL zusammen
	targetURL := "https://" + targetHost + "/" + targetPath

	// Der Rest des Codes (Anfrage stellen, Header/Body kopieren) bleibt identisch.
	proxyReq, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		log.Printf("Tiles Proxy Error: Failed to create request for %s: %v", targetURL, err)
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}
	proxyReq.Header.Set("Origin", c.Request.Host)
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Tiles Proxy Error: Failed to fetch from CDN %s: %v", targetURL, err)
		c.String(http.StatusBadGateway, "Could not reach CDN")
		return
	}
	defer resp.Body.Close()

	c.Writer.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	c.Writer.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		c.Writer.Header().Set("Cache-Control", cacheControl)
	}

	c.Writer.WriteHeader(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}
