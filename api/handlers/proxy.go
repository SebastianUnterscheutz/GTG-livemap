// api/handlers/proxy.go

package handlers

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// The only domain our proxy is allowed to connect to.
const allowedProxyHost = "gtg-cdn.fsn1.your-objectstorage.com"

// CDNProxyHandler is a reverse proxy that safely forwards requests to the CDN.
func CDNProxyHandler(c *gin.Context) {
	// 1. Extract the requested path (e.g. "/map/bootstrap.min.css")
	path := c.Param("path")

	// 2. Security check: Prevent path traversal attacks
	if strings.Contains(path, "..") {
		c.String(http.StatusBadRequest, "Invalid path")
		return
	}

	// 3. Construct the complete target URL
	targetURL := "https://" + allowedProxyHost + path

	// 4. Create a new request to the CDN
	proxyReq, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		log.Printf("Proxy Error: Failed to create request for %s: %v", targetURL, err)
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	// 5. Copy the "Origin" header if present (good practice for some CDNs)
	proxyReq.Header.Set("Origin", c.Request.Host)

	// 6. Execute the request
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Proxy Error: Failed to fetch from CDN %s: %v", targetURL, err)
		c.String(http.StatusBadGateway, "Could not reach CDN")
		return
	}
	defer resp.Body.Close()

	// 7. Copy the headers from the CDN to the response to the user.
	// This is crucial for the correct Content-Type and caching!
	c.Writer.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	c.Writer.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		c.Writer.Header().Set("Cache-Control", cacheControl)
	}

	// 8. Write the status code from the CDN to the response.
	c.Writer.WriteHeader(resp.StatusCode)

	// 9. Stream the body (the actual file) from the CDN directly to the user.
	io.Copy(c.Writer, resp.Body)
}

var allowedProxyHosts = map[string]bool{
	"gtg-cdn.fsn1.your-objectstorage.com": true,
	"reforger.recoil.org":                 true,
}

// TilesProxyHandler is an intelligent reverse proxy for multiple tile sources.
func TilesProxyHandler(c *gin.Context) {
	// 1. Extract the entire path, which now contains "hostname/rest/of/the/path"
	path := c.Param("path")
	if path == "" {
		c.String(http.StatusBadRequest, "Invalid path")
		return
	}

	// Remove the leading "/" if present
	path = strings.TrimPrefix(path, "/")

	// 2. Split the path into hostname and the actual tile path
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		c.String(http.StatusBadRequest, "Invalid proxy path format")
		return
	}
	targetHost := parts[0]
	targetPath := parts[1]

	// 3. ★★★ IMPORTANT SECURITY CHECK ★★★
	// Check if the requested target domain is in our whitelist.
	if !allowedProxyHosts[targetHost] {
		log.Printf("Proxy Warning: Denied request to untrusted host: %s", targetHost)
		c.String(http.StatusForbidden, "Proxying to this host is not allowed")
		return
	}

	// 4. Security check for path traversal
	if strings.Contains(targetPath, "..") {
		c.String(http.StatusBadRequest, "Invalid path")
		return
	}

	// 5. Construct the dynamic target URL
	targetURL := "https://" + targetHost + "/" + targetPath

	// The rest of the code (making the request, copying headers/body) remains identical.
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
