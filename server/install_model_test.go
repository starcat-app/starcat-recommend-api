// Package server 验证外部用户用 ZIP 安装模型的真实脚本及 HTTP 边界，全程仅使用临时实例。
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallModelScript 验证校验失败零激活、正确 ZIP 可用发布密钥激活且可回读。
func TestInstallModelScript(t *testing.T) {
	for _, executable := range []string{"bash", "curl", "jq"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skipf("script prerequisite unavailable: %s", executable)
		}
	}
	service, err := New(Options{
		APIKeys: []string{trainedPublicKey}, ModelPublishKeys: []string{trainedAdminKey},
		ModelRegistryDir: t.TempDir(), SkipListenLogEndpoints: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	host := httptest.NewServer(service.Handler())
	defer host.Close()
	archive := buildBundleZip(t, "installer-v1")
	archivePath := filepath.Join(t.TempDir(), "model.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(checksum string) ([]byte, error) {
		command := exec.Command("bash", "../scripts/install-model.sh", "--version", "installer-v1",
			"--file", archivePath, "--sha256", checksum, "--server", host.URL)
		// 隔离用户本机凭据和服务地址，测试只能指向当前 httptest。
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "MODEL_PUBLISH_KEY=" + trainedAdminKey}
		return command.CombinedOutput()
	}
	if output, err := run(strings.Repeat("0", 64)); err == nil || !strings.Contains(string(output), "checksum mismatch") {
		t.Fatalf("incorrect checksum accepted: %s, %v", output, err)
	}
	if info, err := service.registry.ModelInfo(); err != nil || info != nil {
		t.Fatalf("failed install changed active: %#v, %v", info, err)
	}
	checksum := sha256.Sum256(archive)
	if output, err := run(hex.EncodeToString(checksum[:])); err != nil ||
		!strings.Contains(string(output), "Activated model: installer-v1") {
		t.Fatalf("valid install failed: %s, %v", output, err)
	}
	if info, err := service.registry.ModelInfo(); err != nil || info == nil || info.ModelVersion != "installer-v1" {
		t.Fatalf("installed version mismatch: %#v, %v", info, err)
	}
}
