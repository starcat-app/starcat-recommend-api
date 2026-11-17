// Package handler 提供活动模型的公开读取契约，与管理发布凭据隔离。
package handler

import (
	"net/http"

	"github.com/starcat-app/starcat-recommend-api/internal/serving"
	"github.com/starcat-app/starcat-recommend-api/internal/version"
)

// HandleVersion 返回构建版本和当前模型；不得把目录外查询的内容摘要当成基础模型版本。
func HandleVersion(registry *serving.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		info, err := registry.ModelInfo()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to inspect active model", nil)
			return
		}
		result := struct {
			Service struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"service"`
			Model *serving.ModelInfo `json:"model"`
		}{Model: info}
		result.Service.Name = version.Service
		result.Service.Version = version.Version
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, result)
	}
}
