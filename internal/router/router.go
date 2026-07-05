package router

import (
	"BeeLinkIM/internal/dto"
	"BeeLinkIM/internal/handler"
	"BeeLinkIM/internal/middleware"
	"BeeLinkIM/internal/service"
	"BeeLinkIM/pkg/config"

	"github.com/gin-gonic/gin"
)

type HandlerFunc func(c *gin.Context) dto.Resp

// 3. 封装统一响应中间件（自动把 Resp 转成 JSON）
func GlobalResp(handler HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := handler(c)
		c.JSON(resp.Code, resp)
	}
}
func InitRouter(cfg *config.Config, chatService *service.ChatService) *gin.Engine {
	// 生产模式
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	// 全局中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())
	r.Use(middleware.OnlineCountMiddleware())

	// 注册接口
	r.GET("/health", handler.HealthCheck)    // 健康检查
	r.GET("/online", handler.GetOnlineCount) // 在线人数
	r.GET("/search", handler.Search)
	r.POST("/login", GlobalResp(handler.UserLogin))
	r.POST("/upload", handler.UploadFile)

	// 业务路由组
	api := r.Group("/api/v1")
	{
		api.GET("/hello", func(c *gin.Context) {
			c.JSON(200, gin.H{"msg": "hello production"})
		})
	}

	// 创建路由分组：需要 JWT 鉴权的接口
	authGroup := r.Group("/chat")

	// 注册 JWT 鉴权中间件：所有该组接口必须登录才能访问
	authGroup.Use(middleware.JWTAuth())
	{
		// 注册 WebSocket 接口：客户端通过 /ws 建立长连接
		authGroup.GET("/ws", chatService.ServeWs)
	}
	return r
}
