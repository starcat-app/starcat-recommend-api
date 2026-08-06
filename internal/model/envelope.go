// Package model 定义 Envelope 统一响应结构。
//
// 类型来自 starcat-api-kit/envelope，本文件只做别名以保持现有 import 稳定。
package model

import "github.com/starcat-app/starcat-api-kit/envelope"

// Envelope 是 /api/v1/* 200 响应的顶层包装。
type Envelope[T any] = envelope.Envelope[T]

// Meta 可选的分页/缓存/来源元数据。
type Meta = envelope.Meta

// ErrorResponse 统一错误响应体。
type ErrorResponse = envelope.ErrorResponse

// ErrorEnvelope 所有非 2xx 响应的顶层包装。
type ErrorEnvelope = envelope.ErrorEnvelope
