package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 定义结构体，绑定查询参数
type SearchQueryParam struct {
	Keyword string `form:"keyword" binding:"required"`      // 映射 ?keyword=
	Page    int    `form:"page" binding:"required,min=1"`   // 页码
	Size    int    `form:"size" binding:"omitempty,max=50"` // 非必填，最大50
}

// Search GET 查询参数映射
func Search(c *gin.Context) {
	var param SearchQueryParam
	// 绑定查询参数（自动适配 GET/POST 表单）
	if err := c.ShouldBind(&param); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误：" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": param,
	})
}
