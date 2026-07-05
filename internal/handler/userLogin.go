package handler

import (
	"BeeLinkIM/internal/dto"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 定义结构体，绑定 JSON 参数
type UserLoginParam struct {
	Username string `json:"username" binding:"required"` // 映射json的username
	Password string `json:"password" binding:"required,min=6"`
	Email    string `json:"email" binding:"omitempty,email"` // 可选，邮箱格式
}

// UserLogin POST JSON 参数映射
func UserLogin(c *gin.Context) dto.Resp {
	var param UserLoginParam
	// 绑定 JSON 参数
	if err := c.ShouldBindJSON(&param); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误：" + err.Error(),
		})
		return dto.Resp{Code: 400, Message: "参数错误：" + err.Error()}
	}

	return dto.Resp{Code: 200, Message: "登录成功", Username: param.Username}
}
