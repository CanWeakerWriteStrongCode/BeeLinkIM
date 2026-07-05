package errorx

import (
	"errors"
	"fmt"
)

// 全局错误码定义（生产环境核心！所有业务错误统一在这里管理）
const (
	// 系统级错误
	CodeSuccess      = 0    // 成功
	CodeServerError  = 1000 // 服务器错误
	CodeInvalidParam = 1001 // 参数错误
	CodeUnauthorized = 1002 // 未授权/未登录
	CodeForbidden    = 1003 // 无权限

	// 聊天业务错误
	CodeRoomNotFound    = 2001 // 房间不存在
	CodeMessageSendFail = 2002 // 消息发送失败
	CodeLockAcquireFail = 2003 // 分布式锁获取失败
	CodeUserOffline     = 2004 // 用户不在线
)

// BizError 自定义业务错误（包含 错误码 + 错误信息）
type BizError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// Error 实现 Go 原生 error 接口
func (e *BizError) Error() string {
	return fmt.Sprintf("code: %d, msg: %s", e.Code, e.Msg)
}

// ----------------------------------------------------------------
// 构造函数（对外提供快速创建错误的方法）
// ----------------------------------------------------------------

// New 新建业务错误（推荐）
func New(code int, msg string) error {
	return &BizError{
		Code: code,
		Msg:  msg,
	}
}

// Newf 格式化错误信息
func Newf(code int, format string, args ...interface{}) error {
	return &BizError{
		Code: code,
		Msg:  fmt.Sprintf(format, args...),
	}
}

// ----------------------------------------------------------------
// 工具方法：判断错误类型（生产排查必备）
// ----------------------------------------------------------------

// IsBizError 判断是否为自定义业务错误
func IsBizError(err error) bool {
	_, ok := err.(*BizError)
	return ok
}

// GetCode 获取错误码（如果不是自定义错误，返回服务器错误码）
func GetCode(err error) int {
	if err == nil {
		return CodeSuccess
	}
	if e, ok := err.(*BizError); ok {
		return e.Code
	}
	return CodeServerError
}

// GetMsg 获取错误信息
func GetMsg(err error) string {
	if err == nil {
		return "success"
	}
	if e, ok := err.(*BizError); ok {
		return e.Msg
	}
	return err.Error()
}

// ----------------------------------------------------------------
// 包装原生错误（兼容 Go 标准库 errors）
// ----------------------------------------------------------------
func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
