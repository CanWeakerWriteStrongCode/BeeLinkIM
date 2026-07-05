// 包声明：中间件层
// 作用：统一处理全局横切逻辑（鉴权、日志、限流），不侵入业务代码
package middleware

// 导入依赖包
import (
	"BeeLinkIM/pkg/jwt"    // 自定义JWT工具包：生成/解析Token
	"BeeLinkIM/pkg/logger" // 全局日志：记录鉴权失败日志
	"net/http"             // HTTP状态码：401未授权
	"strings"              // 字符串切割：解析Bearer Token

	"github.com/gin-gonic/gin" // Gin框架：中间件标准规范
	"go.uber.org/zap"          // 结构化日志
)

// 作用：定义JWT鉴权中间件
// 返回值：gin.HandlerFunc → Gin框架中间件的**标准固定格式**
func JWTAuth() gin.HandlerFunc {
	// 返回一个匿名函数：这是Gin中间件的**核心执行逻辑**
	// context *gin.Context：Gin的上下文，承载请求/响应/共享数据
	return func(ginContext *gin.Context) {
		// ====================== 步骤1：从请求中获取Token ======================
		// 定义变量存储最终解析出的Token
		var token string
		// 优先从【请求头Authorization】获取Token
		// 标准规范：HTTP接口鉴权用 Header: Authorization: Bearer <token>
		authHeader := ginContext.GetHeader("Authorization")

		// 如果请求头不为空，解析Bearer Token
		if authHeader != "" {
			// 按空格切割字符串，最多切2段
			// 目的：把 "Bearer xxxToken" 切成 ["Bearer", "xxxToken"]
			parts := strings.SplitN(authHeader, " ", 2)
			// 校验格式：必须是2段，且第一段是Bearer（标准规范）
			if len(parts) == 2 && parts[0] == "Bearer" {
				// 提取第二段：真正的Token字符串
				token = parts[1]
			}
		} else {
			// 如果请求头没有Token，从【URL查询参数】获取
			// ✅ 核心原因：WebSocket握手时，浏览器无法灵活自定义Authorization头
			// 所以前端连接/ws时，用 ?token=xxx 传递，兼容WebSocket场景
			token = ginContext.Query("token")
		}

		// ====================== 步骤2：Token为空 → 鉴权失败 ======================
		if token == "" {
			// 记录警告日志：生产环境排查问题用
			logger.Warn("auth failed: token empty")
			// 返回401未授权状态码 + 提示信息
			ginContext.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录"})
			// ✅ 关键：终止请求，**不执行后续的接口逻辑**（比如/ws连接）
			ginContext.Abort()
			// 退出中间件，不再往下执行
			return
		}

		// ====================== 步骤3：解析Token（校验合法性/过期时间） ======================
		// 调用自定义JWT工具：解析Token，拿到用户信息（claims）
		claims, err := jwt.ParseToken(token)
		// 解析失败：Token过期/伪造/格式错误
		if err != nil {
			logger.Warn("auth failed: parse token error", zap.Error(err))
			ginContext.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "登录已过期"})
			// 终止请求
			ginContext.Abort()
			return
		}

		// ====================== 步骤4：鉴权成功 → 存储用户ID到上下文 ======================
		// ✅ 核心作用：把解析出的用户UID存入Gin上下文
		// 后续接口（如/ws）可以直接通过 ginContext.Get("uid") 获取用户ID
		ginContext.Set("uid", claims.UID)
		// ✅ 关键：继续执行后续的中间件/接口逻辑（放行请求）
		ginContext.Next()
	}
}
