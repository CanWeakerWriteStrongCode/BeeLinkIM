// Package mysqlx 提供 MySQL 数据库访问层的实现
// 负责与聊天系统相关的数据持久化操作，包括聊天室和消息的增删改查
package mysqlx

import (
	"BeeLinkIM/pkg/logger" // 导入项目内部日志包，用于记录结构化日志
	"context"              // 导入上下文包，用于控制请求的生命周期、超时和取消操作

	"go.uber.org/zap" // 导入 Zap 高性能日志框架，提供结构化日志记录
	"gorm.io/gorm"    // 导入 GORM ORM 库，提供 Go 语言对象与数据库表之间的映射
)

// MySQLRepo MySQL 数据存储库结构体
// 实现数据访问层接口，封装所有与 MySQL 交互的逻辑
// 采用依赖注入模式，通过构造函数传入 *gorm.DB 实例
type MySQLRepo struct {
	db *gorm.DB // 私有字段：GORM 数据库连接实例，作用域为当前结构体，用于执行所有 SQL 操作
}

// NewMySQLRepo MySQL 仓库构造函数
// 参数 db: GORM 数据库连接实例（作用域：调用方传入）
// 返回：*MySQLRepo 初始化的仓库指针
// 设计意图：遵循工厂函数模式，统一初始化逻辑，便于后续扩展（如连接池配置、中间件注册等）
func NewMySQLRepo(db *gorm.DB) *MySQLRepo {
	return &MySQLRepo{db: db} // 返回初始化后的仓库实例，将传入的 db 赋值给结构体字段
}

// GetOrCreateRoom 获取或创建聊天房间方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制数据库查询超时和取消
// 参数 uid1, uid2: 两个用户的 ID（作用域：调用方传入），用于确定聊天房间的唯一性
// 返回：*TChatRoom 聊天房间对象，error 错误信息
// 业务逻辑：采用"先查后创"策略，确保同一对用户的聊天房间唯一存在
// 设计意图：支持单聊场景，房间由两个用户 ID 唯一确定，不考虑顺序（即 uid1,uid2 和 uid2,uid1 视为同一房间）
func (r *MySQLRepo) GetOrCreateRoom(ctx context.Context, uid1, uid2 int64) (*TChatRoom, error) {
	// 调用辅助函数确保 uid1 < uid2，保证房间 ID 的顺序一致性
	// 原因：数据库中唯一键基于 (uid1, uid2) 且要求 uid1 < uid2，避免重复创建
	a, b := minMax(uid1, uid2)           // a: 较小的用户 ID, b: 较大的用户 ID（作用域：当前方法）
	room := &TChatRoom{UID1: a, UID2: b} // 初始化房间对象，设置两个用户 ID（作用域：当前方法）

	// 使用 GORM 在数据库中查询房间记录
	// WithContext: 绑定上下文，支持超时控制和请求追踪
	// Where: 指定查询条件，使用预编译语句防止 SQL 注入
	// First: 查询第一条匹配记录，若未找到返回 gorm.ErrRecordNotFound 错误
	err := r.db.WithContext(ctx).Where("uid1 = ? AND uid2 = ?", a, b).First(room).Error
	if err == nil {
		return room, nil // 查询成功：直接返回找到的房间对象，nil 表示无错误
	}

	// 检查错误类型：如果不是"记录未找到"错误，说明是真正的数据库错误
	if err != gorm.ErrRecordNotFound {
		logger.Error("query room failed", zap.Error(err)) // 记录错误日志，包含具体错误堆栈
		return nil, err                                   // 返回 nil 房间和原始错误，终止方法执行
	}

	// 执行到这里说明房间不存在（err == gorm.ErrRecordNotFound）
	// 使用 GORM 的 Create 方法插入新房间记录
	err = r.db.WithContext(ctx).Create(room).Error
	if err != nil {
		// 处理唯一键冲突场景：并发请求可能同时创建相同房间
		// 此时忽略错误，因为另一个请求已经创建了房间
		// 再次查询获取已存在的房间记录（忽略查询错误，因为理论上应该存在）
		_ = r.db.WithContext(ctx).Where("uid1 = ? AND uid2 = ?", a, b).First(room).Error
		return room, nil // 返回房间对象（可能是刚创建的或因冲突已存在的），视为成功
	}
	return room, nil // 创建成功：返回新创建的房间对象，nil 表示无错误
}

// CreateMessage 创建并保存消息记录方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制数据库操作的生命周期
// 参数 msg: 消息对象指针（作用域：调用方传入），包含消息内容、发送者、房间 ID 等信息
// 返回：error 错误信息，nil 表示保存成功
// 设计意图：将聊天消息持久化到 MySQL 数据库，支持后续历史消息查询
func (r *MySQLRepo) CreateMessage(ctx context.Context, msg *TChatMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error // 使用 GORM 创建消息记录，返回错误（有则非 nil，无则 nil）
}

// minMax 辅助函数：确保返回的两个整数按升序排列
// 参数 a, b: 输入的两个整数（作用域：函数调用期间）
// 返回：(较小值，较大值)，保证第一个返回值 <= 第二个返回值
// 设计意图：用于规范化用户 ID 顺序，确保数据库查询条件的一致性
func minMax(a, b int64) (int64, int64) {
	if a < b {
		return a, b // a 较小，保持原顺序返回
	}
	return b, a // b 较小或相等，交换顺序返回，确保第一个值始终是最小值
}
