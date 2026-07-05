// 包声明：JWT 工具包
// 作用：封装 JWT Token 的【生成】和【解析】功能，全局复用
package jwt

// 导入依赖包
import (
	"BeeLinkIM/pkg/config" // 全局配置：读取 JWT 签名密钥
	"errors"               // 自定义错误：返回无效 Token 提示
	"time"                 // 时间：设置 Token 过期时间

	"github.com/golang-jwt/jwt/v5"
)

// -------------------------- 1. 自定义 JWT 声明结构体 --------------------------
// CustomClaims
// JWT 核心数据载体，分为【自定义字段】+【标准字段】
// 作用：存储用户身份信息（UID），加密后生成 Token
type CustomClaims struct {
	// 自定义字段：用户唯一ID（聊天系统必须用UID标识用户）
	// 前端登录后，所有请求通过 Token 携带 UID，服务端解析识别用户
	UID int64 `json:"uid"`

	// 嵌套 JWT 标准声明（官方固定字段，必须包含）
	// 标准字段：过期时间、签发时间、签发者等，保证 Token 合法性
	jwt.RegisteredClaims
}

// -------------------------- 2. 生成 JWT Token（登录时调用） --------------------------
// GenerateToken
// 输入：用户ID（UID）
// 输出：加密后的 Token 字符串 / 错误
// 场景：用户登录成功后，调用此函数生成 Token 返回给前端
func GenerateToken(uid int64) (string, error) {
	// 获取当前时间
	nowTime := time.Now()
	// 设置 Token 过期时间：7 天
	// 原因：聊天系统属于长期在线应用，7 天过期兼顾安全性和用户体验
	expireTime := nowTime.Add(7 * 24 * time.Hour)

	// 构造 JWT 声明数据
	claims := CustomClaims{
		// 自定义：存入用户ID，后续解析直接拿到当前登录用户
		UID: uid,
		// 标准声明（必填，保证 Token 合规）
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime), // Token 过期时间
			IssuedAt:  jwt.NewNumericDate(nowTime),    // Token 签发时间
			Issuer:    "BeeLinkIM",                    // 签发者（自定义项目名）
		},
	}

	// 创建 Token 对象
	// jwt.SigningMethodHS256：使用【HS256 对称加密算法】签名
	// 优点：加密速度快、实现简单，适合分布式/集群系统（所有节点共享密钥）
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 用【配置文件中的密钥】对 Token 进行签名，生成最终的字符串 Token
	// 密钥从全局配置读取，而非硬编码：生产环境安全规范，方便更换密钥
	return token.SignedString([]byte(config.GlobalConfig.App.JWTSecret))
}

// -------------------------- 3. 解析 JWT Token（鉴权时调用） --------------------------
// ParseToken
// 输入：前端传来的 Token 字符串
// 输出：解析后的用户信息（CustomClaims）/ 错误
// 场景：中间件 JWTAuth 中调用，验证 Token 是否合法、是否过期
func ParseToken(tokenString string) (*CustomClaims, error) {
	// 解析 Token
	// 1. 传入 Token 字符串
	// 2. 传入自定义声明结构体（用于接收解析后的数据）
	// 3. 回调函数：返回签名密钥，用于验证 Token 未被篡改
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 返回配置中的 JWT 密钥，和生成时的密钥一致
		return []byte(config.GlobalConfig.App.JWTSecret), nil
	})

	// 解析失败：Token 格式错误、过期、签名不匹配
	if err != nil {
		return nil, err
	}

	// 类型断言：将解析后的通用声明 转为 我们自定义的 CustomClaims
	// 同时校验 token.Valid：Token 是否合法（未过期、签名正确）
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		// 解析成功：返回包含 UID 的声明对象
		return claims, nil
	}

	// 校验失败：返回自定义错误
	return nil, errors.New("invalid token")
}
