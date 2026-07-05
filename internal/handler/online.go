package handler

import (
	"BeeLinkIM/internal/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetOnlineCount 获取在线人数
func GetOnlineCount(c *gin.Context) {
	count := middleware.OnlineCount.Load()
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"online_count": count,
			"desc":         "当前活跃连接数",
		},
	})
}
