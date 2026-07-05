package middleware

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// 全局原子计数器（并发安全）
var OnlineCount atomic.Int64

// OnlineCountMiddleware 在线人数统计
func OnlineCountMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		OnlineCount.Add(1)
		defer OnlineCount.Add(-1)
		c.Next()
	}
}
