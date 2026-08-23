package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/starcat-app/starcat-recommend-api/internal/serving"
)

// BundleHandler 接收 Trainer 发布的内部模型包。
type BundleHandler struct {
	registry     *serving.Registry
	maximumBytes int64
}

// NewBundleHandler 创建带请求体硬上限的发布 handler。
func NewBundleHandler(registry *serving.Registry, maximumBytes int64) *BundleHandler {
	return &BundleHandler{registry: registry, maximumBytes: maximumBytes}
}

// HandleUpload 校验、不可变安装并按参数激活 Bundle。
func (h *BundleHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/zip" {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/zip", nil)
		return
	}
	activate, err := strconv.ParseBool(r.URL.Query().Get("activate"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "activate must be true or false", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maximumBytes)
	bundle, err := h.registry.InstallZip(r.Context(), r.PathValue("model_version"), r.Body, activate)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		switch {
		case errors.As(err, &maxBytesError):
			writeError(w, http.StatusRequestEntityTooLarge, "BUNDLE_TOO_LARGE", "bundle exceeds size limit", nil)
		case errors.Is(err, serving.ErrVersionConflict):
			writeError(w, http.StatusConflict, "VERSION_CONFLICT", "model version already exists with different content", nil)
		case errors.Is(err, serving.ErrInvalidBundle):
			writeError(w, http.StatusUnprocessableEntity, "INVALID_BUNDLE", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "PUBLISH_FAILED", "bundle publish failed", nil)
		}
		return
	}
	writeJSON(w, struct {
		ModelVersion  string `json:"model_version"`
		SelectedModel string `json:"selected_model"`
		Active        bool   `json:"active"`
	}{bundle.Version, bundle.Manifest.SelectedModel, activate})
}

// HandleActive 返回内部发布方可见的 active 模型状态。
func (h *BundleHandler) HandleActive(w http.ResponseWriter, _ *http.Request) {
	bundle, err := h.registry.Active()
	if err != nil {
		writeServingError(w, err)
		return
	}
	writeJSON(w, struct {
		ModelVersion  string `json:"model_version"`
		SelectedModel string `json:"selected_model"`
	}{bundle.Version, bundle.Manifest.SelectedModel})
}
