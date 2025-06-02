package api

import (
	"augment2api/config"
	"augment2api/pkg/logger"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 验证请求的Authorization header并设置token信息
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 调试模式下跳过认证检查
		if config.AppConfig.CodingMode == "true" {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Log.Error("Authorization is empty")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		// 支持 "Bearer " 格式
		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		// 如果设置了固定的AuthToken，则验证token是否匹配
		if config.AppConfig.AuthToken != "" {
			if token != config.AppConfig.AuthToken {
				logger.Log.Error(fmt.Sprintf("Invalid authorization token:%s", token))
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization token"})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// 如果没有设置固定AuthToken，则验证token是否存在于Redis中
		tokenKey := "token:" + token
		tenantURL, err := config.RedisHGet(tokenKey, "tenant_url")
		if err != nil || tenantURL == "" {
			logger.Log.Error(fmt.Sprintf("Invalid or non-existent token: %s", token))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization token"})
			c.Abort()
			return
		}

		// 将验证通过的token和tenant_url设置到上下文中
		c.Set("token", token)
		c.Set("tenant_url", tenantURL)
		c.Next()
	}
}