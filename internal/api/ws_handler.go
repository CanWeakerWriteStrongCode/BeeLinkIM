// 包声明：API 接口层
// 作用：专门处理外部 HTTP/WebSocket 请求，对接业务服务层（service）
package api

// 导入依赖包
import (
	"BeeLinkIM/internal/ws"
	"net/http"

	"github.com/gorilla/websocket"
)

// -------------------------- 2. WebSocket 处理器结构体 --------------------------
// 作用：处理所有 /ws 连接请求，通过**依赖注入**持有业务核心组件
// 解耦设计：不直接创建对象，外部传入，方便测试和维护
type WsHandler struct {
	Upgrader *websocket.Upgrader // 协议升级器：将 HTTP 升级为 WebSocket
}

// 构造函数：创建 WebSocket 处理器实例
// 标准依赖注入：外部传入 Hub 和 ChatService
func NewWsHandler(h *ws.Hub) *WsHandler {
	// -------------------------- 1. 全局 WebSocket 协议升级器 --------------------------
	// 全局单例：将 HTTP 协议 升级为 WebSocket 长连接的核心工具
	// 全局定义：避免每次请求都创建，节省内存，提升性能
	var upgrader = websocket.Upgrader{
		// 读缓冲区大小：1024 字节
		// 作用：缓存客户端发送过来的消息，生产环境根据消息大小调整
		ReadBufferSize: 1024,
		// 写缓冲区大小：1024 字节
		// 作用：缓存服务端发送给客户端的消息
		WriteBufferSize: 1024,
		// 跨域检查：Websocket 强制校验请求来源（Origin）
		CheckOrigin: func(r *http.Request) bool {
			// 返回 true：允许所有跨域请求
			// 生产环境：可以改成校验域名，提升安全性
			// 原因：前端页面和后端接口通常跨域，必须开启否则连接失败
			return true
		},
	}
	return &WsHandler{Upgrader: &upgrader}
}
