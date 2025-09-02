package middleware

import (
	"net/http"
	"strings"

	"auction_service/internal/config"

	"github.com/gin-gonic/gin"
)

func CORS(cfg *config.Config) gin.HandlerFunc {
	allowedOrigins := strings.Split(cfg.CORSAllowedOrigins, ",")
	allowedMethods := cfg.CORSAllowedMethods
	allowedHeaders := cfg.CORSAllowedHeaders

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		
		if origin != "" && (cfg.CORSAllowedOrigins == "*" || contains(allowedOrigins, origin)) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		} else if cfg.CORSAllowedOrigins == "*" {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		
		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Allow-Credentials", "true")
		
		// WebSocket specific headers
		c.Header("Access-Control-Allow-Headers", allowedHeaders+",Sec-WebSocket-Extensions,Sec-WebSocket-Key,Sec-WebSocket-Version")
		c.Header("Access-Control-Expose-Headers", "Sec-WebSocket-Accept")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		
		c.Next()
	}
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == target {
			return true
		}
	}
	return false
}