// Package redisx 提供 Redis 数据访问层的实现
// 负责聊天系统的缓存管理、分布式锁、用户路由和房间会话存储
package redisx

import (
	"context" // 导入上下文包，用于控制请求的生命周期、超时和取消操作
	"fmt"     // 导入格式化包，用于字符串格式化（如生成 Redis key）
	"strconv" // 导入字符串转换包，用于数字与字符串之间的转换
	"time"    // 导入时间包，用于设置过期时间和延时操作

	"github.com/bsm/redislock"     // 导入基于 Redis 的分布式锁实现库，提供跨进程的互斥锁
	"github.com/redis/go-redis/v9" // 导入 Redis Go 客户端 v9 版本，提供高性能的 Redis 操作接口
)

// RedisRepo Redis 数据存储库结构体
// 实现数据访问层接口，封装所有与 Redis 交互的逻辑
// 采用依赖注入模式，通过构造函数传入 Redis 客户端和分布式锁客户端
type RedisRepo struct {
	rdb    *redis.Client     // 私有字段：Redis 客户端实例（作用域：当前结构体），用于执行所有 Redis 命令
	locker *redislock.Client // 私有字段：分布式锁客户端实例（作用域：当前结构体），用于获取和释放分布式锁
}

// RoomSession Redis 缓存的房间会话数据结构
// 用于在内存中快速访问聊天房间的核心信息，避免频繁查询 MySQL
// 适用场景：高频访问的房间元数据，如房间 ID 和消息序列号
type RoomSession struct {
	RoomID   int64 // 公共字段：聊天房间的唯一标识（作用域：公开），对应 MySQL 中的 TChatRoom.ID
	Sequence int64 // 公共字段：房间内消息的序列号（作用域：公开），用于保证消息的顺序性和去重
}

// NewRedisRepo Redis 仓库构造函数
// 参数 rdb: Redis 客户端实例（作用域：调用方传入），用于数据缓存操作
// 参数 locker: 分布式锁客户端实例（作用域：调用方传入），用于并发控制
// 返回：*RedisRepo 初始化的仓库指针
// 设计意图：遵循工厂函数模式，统一初始化逻辑，便于依赖注入和单元测试
func NewRedisRepo(rdb *redis.Client, locker *redislock.Client) *RedisRepo {
	return &RedisRepo{rdb: rdb, locker: locker} // 返回初始化后的仓库实例，将传入的参数赋值给结构体字段
}

// ---------------- 分布式锁 ----------------

// UnlockFunc 解锁函数的类型定义
// 无参数，无返回值
// 设计意图：将解锁操作封装为闭包，延迟执行，确保锁的正确释放（即使在 panic 场景下）
type UnlockFunc func()

// GetLock 获取分布式锁方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制锁超时的等待时间
// 参数 key: 锁的唯一标识键（作用域：调用方传入），通常使用业务相关的唯一字符串
// 返回：UnlockFunc 解锁函数（作用域：调用方），error 错误信息
// 业务逻辑：使用 redislock 库获取分布式锁，超时时间 5 秒
// 设计意图：防止并发场景下的资源竞争，如同时创建相同房间、重复发送消息等
// 性能考量：锁超时 5 秒自动释放，避免死锁；返回解锁函数由调用方 defer 调用，确保及时释放
func (r *RedisRepo) GetLock(ctx context.Context, key string) (UnlockFunc, error) {
	// 尝试获取分布式锁，锁键前缀 "lock:" 用于区分普通数据键
	// Obtain 参数：ctx 上下文、锁键、锁超时时间（5 秒）、可选配置（nil 表示默认）
	lock, err := r.locker.Obtain(ctx, "lock:"+key, 5*time.Second, nil)
	if err != nil {
		return nil, err // 获取锁失败（可能被其他进程持有或超时），返回 nil 解锁函数和错误
	}
	// 返回解锁闭包函数，该函数会在调用时释放锁
	return func() {
		_ = lock.Release(context.Background()) // 释放锁，忽略错误（因为锁可能已过期自动释放）
	}, nil
}

// ---------------- 用户路由 (User -> Server) ----------------

// SetUserServer 设置用户所在的服务节点方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制操作超时
// 参数 uid: 用户唯一标识（作用域：调用方传入），int64 类型
// 参数 serverID: 服务器节点标识（作用域：调用方传入），通常为服务器 IP 或主机名
// 返回：error 错误信息，nil 表示设置成功
// 业务逻辑：将用户 ID 与当前连接的 WebSocket 服务器绑定，用于后续消息路由
// 设计意图：在分布式架构中，快速定位用户所在的服务器节点，实现跨节点消息投递
// 过期策略：24 小时自动过期，适用于用户长期在线的场景，离线后自动清理路由信息
func (r *RedisRepo) SetUserServer(ctx context.Context, uid int64, serverID string) error {
	// 使用 SET 命令存储用户 - 服务器映射关系
	// Key 格式："user:server:{uid}"，Value: serverID，过期时间：24 小时
	return r.rdb.Set(ctx, fmt.Sprintf("user:server:%d", uid), serverID, 24*time.Hour).Err()
}

// GetUserServer 获取用户所在的服务节点方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制操作超时
// 参数 uid: 用户唯一标识（作用域：调用方传入），int64 类型
// 返回：string 服务器节点标识，error 错误信息
// 业务逻辑：从 Redis 缓存中查询用户当前连接的服务器节点
// 设计意图：配合 SetUserServer 使用，实现消息的精准路由（单播到目标用户所在服务器）
func (r *RedisRepo) GetUserServer(ctx context.Context, uid int64) (string, error) {
	// 使用 GET 命令获取用户 - 服务器映射关系
	// Key 格式："user:server:{uid}"，若不存在则返回空字符串和 redis.Nil 错误
	return r.rdb.Get(ctx, fmt.Sprintf("user:server:%d", uid)).Result()
}

// ---------------- 房间会话 ----------------

// buildRoomKey 构建房间会话的 Redis 键辅助函数
// 参数 uid1, uid2: 两个用户的 ID（作用域：函数调用期间），用于确定房间的唯一性
// 返回：string 格式化的 Redis 键字符串
// 设计意图：统一房间键的命名规范，确保相同用户对生成的键一致（不依赖顺序）
// 原因：房间由两个用户 ID 唯一确定，需要规范化顺序以避免 "uid1,uid2" 和 "uid2,uid1" 产生不同的键
func buildRoomKey(uid1, uid2 int64) string {
	a, b := minMax(uid1, uid2)                     // 调用辅助函数确保 a < b，保证键的一致性
	return fmt.Sprintf("room:session:%d-%d", a, b) // 生成键格式："room:session:小 ID-大 ID"，例如 "room:session:1001-1002"
}

// GetRoomSession 获取房间会话信息方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制操作超时
// 参数 uid1, uid2: 两个用户的 ID（作用域：调用方传入），用于确定房间
// 返回：*RoomSession 房间会话对象指针，error 错误信息
// 业务逻辑：从 Redis Hash 结构中读取房间的缓存数据（房间 ID 和消息序列号）
// 设计意图：快速获取房间元数据，避免频繁查询 MySQL，提升性能
func (r *RedisRepo) GetRoomSession(ctx context.Context, uid1, uid2 int64) (*RoomSession, error) {
	key := buildRoomKey(uid1, uid2)              // 调用辅助函数生成标准化的房间键
	res, err := r.rdb.HGetAll(ctx, key).Result() // 使用 HGETALL 命令获取整个 Hash 对象的所有字段和值
	if err != nil || len(res) == 0 {
		return nil, err // 查询失败或房间会话不存在（Hash 为空），返回 nil 和错误
	}

	// 将 Redis Hash 中的字符串值转换为 int64 类型
	// ParseInt 参数：待转换字符串、进制（10 表示十进制）、位宽（64 表示 int64）
	roomID, _ := strconv.ParseInt(res["room_id"], 10, 64)   // 忽略错误，假设数据始终有效（由 SetRoomSession 保证）
	seq, _ := strconv.ParseInt(res["sequence"], 10, 64)     // 忽略错误，假设数据始终有效
	return &RoomSession{RoomID: roomID, Sequence: seq}, nil // 返回解析后的房间会话对象，nil 表示无错误
}

// SetRoomSession 设置房间会话信息方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制操作超时
// 参数 uid1, uid2: 两个用户的 ID（作用域：调用方传入），用于确定房间
// 参数 s: 房间会话对象指针（作用域：调用方传入），包含房间 ID 和序列号
// 返回：error 错误信息，nil 表示设置成功
// 业务逻辑：将房间元数据写入 Redis Hash 结构，用于后续快速读取
// 设计意图：配合 GetRoomSession 使用，实现房间信息的缓存加速
func (r *RedisRepo) SetRoomSession(ctx context.Context, uid1, uid2 int64, s *RoomSession) error {
	key := buildRoomKey(uid1, uid2) // 调用辅助函数生成标准化的房间键
	// 使用 HMSET 命令批量设置 Hash 字段
	// 字段："room_id" 和 "sequence"，值为 RoomSession 对象的对应字段
	return r.rdb.HSet(ctx, key, "room_id", s.RoomID, "sequence", s.Sequence).Err()
}

// IncrSequence 递增房间消息序列号方法
// 参数 ctx: 上下文对象（作用域：方法调用期间），用于控制操作超时
// 参数 uid1, uid2: 两个用户的 ID（作用域：调用方传入），用于确定房间
// 返回：int64 递增后的新序列号，error 错误信息
// 业务逻辑：使用 Redis 原子自增操作，为房间内的每条消息生成唯一的递增序号
// 设计意图：保证同一房间内消息的全局有序性，支持消息去重和按序投递
// 性能考量：HIncrBy 是原子操作，天然支持并发安全，无需额外加锁
func (r *RedisRepo) IncrSequence(ctx context.Context, uid1, uid2 int64) (int64, error) {
	key := buildRoomKey(uid1, uid2)                        // 调用辅助函数生成标准化的房间键
	return r.rdb.HIncrBy(ctx, key, "sequence", 1).Result() // 对 Hash 中的 "sequence" 字段执行 +1 操作，返回新值
}

// minMax 辅助函数：确保返回的两个整数按升序排列
// 参数 a, b: 输入的两个整数（作用域：函数调用期间）
// 返回：(较小值，较大值)，保证第一个返回值 <= 第二个返回值
// 设计意图：用于规范化用户 ID 顺序，确保 Redis 键和数据库查询条件的一致性
func minMax(a, b int64) (int64, int64) {
	if a < b {
		return a, b // a 较小，保持原顺序返回
	}
	return b, a // b 较小或相等，交换顺序返回，确保第一个值始终是最小值
}
