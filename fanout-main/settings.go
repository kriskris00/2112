package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// WebSettings 是管理界面自身的可改配置：监听端口、监听地址（本地/全接口）。
// 落盘持久化，界面改完重启监听即时生效。访问口令与访问路径各有专门的文件
// （password / basepath），不放这里，但都能在设置面板里改。
type WebSettings struct {
	// Port 是管理界面监听端口。
	Port int `json:"port"`
	// ListenAddr 是监听地址：空或 0.0.0.0 表示所有网卡；127.0.0.1 表示只本机。
	ListenAddr string `json:"listen_addr"`
}

// ExitNodeLimitSettings 控制出口节点数量上限。
// Mode=country 时按国家限制；Mode=isp 时按“国家+运营商”分别限制。
type ExitNodeLimitSettings struct {
	Enabled bool   `json:"enabled"`
	Limit   int    `json:"limit"`
	Mode    string `json:"mode"`
}

var (
	exitNodeLimitMu   sync.RWMutex
	exitNodeLimitCur  ExitNodeLimitSettings
	exitNodeLimitPath string

	webSettingsMu   sync.RWMutex
	webSettingsCur  WebSettings
	webSettingsPath string
)

func webSettingsFilePath(dir string) string { return filepath.Join(dir, "settings.json") }

// loadWebSettings 读盘并返回当前配置。
//
// portExplicit 表示用户在命令行显式给了 -web。界面上改过端口之后会落盘，
// 之前这里一律以盘上为准，导致再带 -web 启动会被静默忽略——用户敲了参数却
// 连不上，也没有任何提示。显式指定时以命令行为准并写回，让参数说话算话。
func loadWebSettings(dir string, defaultPort int, portExplicit bool) (WebSettings, error) {
	webSettingsPath = webSettingsFilePath(dir)

	s := WebSettings{Port: defaultPort, ListenAddr: ""}
	blob, err := os.ReadFile(webSettingsPath)
	switch {
	case os.IsNotExist(err):
		webSettingsMu.Lock()
		webSettingsCur = s
		webSettingsMu.Unlock()
		return s, saveWebSettings()
	case err != nil:
		return s, err
	}
	if err := json.Unmarshal(blob, &s); err != nil {
		return s, err
	}
	if s.Port == 0 {
		s.Port = defaultPort
	}
	changed := false
	if portExplicit && s.Port != defaultPort {
		s.Port = defaultPort
		changed = true
	}
	webSettingsMu.Lock()
	webSettingsCur = s
	webSettingsMu.Unlock()
	if changed {
		return s, saveWebSettings()
	}
	return s, nil
}

func getWebSettings() WebSettings {
	webSettingsMu.RLock()
	defer webSettingsMu.RUnlock()
	return webSettingsCur
}

func saveWebSettings() error {
	webSettingsMu.RLock()
	blob, err := json.MarshalIndent(webSettingsCur, "", "  ")
	webSettingsMu.RUnlock()
	if err != nil {
		return err
	}
	tmp := webSettingsPath + ".tmp"
	if err := os.WriteFile(tmp, blob, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, webSettingsPath)
}

func exitNodeLimitFilePath(dir string) string { return filepath.Join(dir, "exit_node_limit.json") }

func normalizeExitNodeLimitMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "isp" || mode == "operator" || mode == "provider" {
		return "isp"
	}
	return "country"
}

func loadExitNodeLimitSettings(dir string) (ExitNodeLimitSettings, error) {
	exitNodeLimitPath = exitNodeLimitFilePath(dir)
	s := ExitNodeLimitSettings{Enabled: true, Limit: 5, Mode: "country"}
	blob, err := os.ReadFile(exitNodeLimitPath)
	if os.IsNotExist(err) {
		exitNodeLimitMu.Lock()
		exitNodeLimitCur = s
		exitNodeLimitMu.Unlock()
		return s, saveExitNodeLimitSettings()
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(blob, &s); err != nil {
		return s, err
	}
	if s.Limit < 1 {
		s.Limit = 5
	}
	if s.Limit > 1000 {
		s.Limit = 1000
	}
	s.Mode = normalizeExitNodeLimitMode(s.Mode)
	exitNodeLimitMu.Lock()
	exitNodeLimitCur = s
	exitNodeLimitMu.Unlock()
	return s, nil
}

func getExitNodeLimitSettings() ExitNodeLimitSettings {
	exitNodeLimitMu.RLock()
	defer exitNodeLimitMu.RUnlock()
	return exitNodeLimitCur
}

func saveExitNodeLimitSettings() error {
	exitNodeLimitMu.RLock()
	blob, err := json.MarshalIndent(exitNodeLimitCur, "", "  ")
	exitNodeLimitMu.RUnlock()
	if err != nil {
		return err
	}
	tmp := exitNodeLimitPath + ".tmp"
	if err := os.WriteFile(tmp, blob, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, exitNodeLimitPath)
}

func setExitNodeLimitSettings(next ExitNodeLimitSettings) error {
	if next.Limit < 1 || next.Limit > 1000 {
		return fmt.Errorf("节点限额必须在 1-1000 之间")
	}
	next.Mode = normalizeExitNodeLimitMode(next.Mode)
	exitNodeLimitMu.Lock()
	exitNodeLimitCur = next
	exitNodeLimitMu.Unlock()
	return saveExitNodeLimitSettings()
}

// normalizeListenAddr 把用户填的监听地址规整成合法值：空 / 0.0.0.0 / 127.0.0.1 / 具体 IP。
func normalizeListenAddr(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" || addr == "0.0.0.0" || strings.EqualFold(addr, "all") {
		return "", nil
	}
	if ip := net.ParseIP(addr); ip != nil {
		return addr, nil
	}
	return "", fmt.Errorf("监听地址必须是合法 IP，或留空表示所有网卡")
}

// validatePort 校验端口范围。
func validatePort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("端口必须在 1-65535 之间")
	}
	return nil
}

// listenAddrString 拼出 net.Listen 用的地址串。
func (s WebSettings) listenAddrString() string {
	return net.JoinHostPort(s.ListenAddr, strconv.Itoa(s.Port))
}
