package handler

import (
	"mime/multipart"

	"github.com/gin-gonic/gin"
)

type UploadParam struct {
	File *multipart.FileHeader `form:"file" binding:"required"` // 文件
	Desc string                `form:"desc"`                    // 描述
}

func UploadFile(c *gin.Context) {
	var param UploadParam
	if err := c.ShouldBind(&param); err != nil {
		c.JSON(400, gin.H{"msg": err.Error()})
		return
	}
	// 保存文件
	c.SaveUploadedFile(param.File, "./upload/"+param.File.Filename)
	c.JSON(200, gin.H{"msg": "上传成功"})
}
