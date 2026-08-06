// Package middleware 提供 Bearer Token 鉴权与 CORS。
//
// 实现已收敛到 starcat-api-kit；本包保留原 import 路径，避免业务代码大面积改动。
package middleware

import (
	"net/http"

	kitauth "github.com/starcat-app/starcat-api-kit/auth"
	kitcors "github.com/starcat-app/starcat-api-kit/cors"
)

// BearerAuth 是 kit auth 的类型别名。
type BearerAuth = kitauth.BearerAuth

// NewBearerAuth 创建 Bearer 鉴权中间件。
func NewBearerAuth(keys []string) *BearerAuth {
	return kitauth.NewBearerAuth(keys)
}

// CORS 注入跨域响应头并处理 OPTIONS。
func CORS(next http.Handler) http.Handler {
	return kitcors.Handler(next)
}
