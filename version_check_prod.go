//go:build !localtest

package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/metacubex/mihomo/log"
)

// checkVersion blocks until the remote version is verified.
// Returns nil if update is NOT needed, or an error if update IS needed or check failed.
func checkVersion() error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(VersionURL)
	if err != nil {
		return fmt.Errorf("无法连接版本服务器: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取版本信息失败: %w", err)
	}

	remoteVer, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return fmt.Errorf("版本号解析失败: %w", err)
	}

	log.Infoln("Remote version: %d, Built-in version: %d", remoteVer, BuiltInVersion)
	if remoteVer > BuiltInVersion {
		return fmt.Errorf("发现新版本 v%d，当前版本 v%d，请前往 %s 更新", remoteVer, BuiltInVersion, UpdateURL)
	}
	return nil
}
