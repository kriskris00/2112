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

type SubscriptionSettings struct {
	QuotaGB  float64 `json:"quota_gb"`
	ExpireAt int64   `json:"expire_at"` // Unix milliseconds; 0 = never
}

var subscriptionSettingsMu sync.RWMutex
var subscriptionSettingsCur SubscriptionSettings
var subscriptionSettingsPath string

func loadSubscriptionSettings(dir string) (SubscriptionSettings, error) {
	subscriptionSettingsPath = filepath.Join(dir, "subscription_settings.json")
	s := SubscriptionSettings{}
	blob, err := os.ReadFile(subscriptionSettingsPath)
	if os.IsNotExist(err) {
		subscriptionSettingsMu.Lock()
		subscriptionSettingsCur = s
		subscriptionSettingsMu.Unlock()
		return s, saveSubscriptionSettings()
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(blob, &s); err != nil {
		return s, err
	}
	if s.QuotaGB < 0 {
		s.QuotaGB = 0
	}
	subscriptionSettingsMu.Lock()
	subscriptionSettingsCur = s
	subscriptionSettingsMu.Unlock()
	return s, nil
}

func getSubscriptionSettings() SubscriptionSettings {
	subscriptionSettingsMu.RLock()
	defer subscriptionSettingsMu.RUnlock()
	return subscriptionSettingsCur
}

func saveSubscriptionSettings() error {
	subscriptionSettingsMu.RLock()
	blob, err := json.MarshalIndent(subscriptionSettingsCur, "", "  ")
	subscriptionSettingsMu.RUnlock()
	if err != nil {
		return err
	}
	tmp := subscriptionSettingsPath + ".tmp"
	if err := os.WriteFile(tmp, blob, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, subscriptionSettingsPath)
}

func setSubscriptionSettings(next SubscriptionSettings) error {
	if next.QuotaGB < 0 || next.QuotaGB > 100000 {
		return fmt.Errorf("订阅流量必须在 0-100000 GB 之间，0 表示不限")
	}
	if next.ExpireAt < 0 {
		return fmt.Errorf("订阅到期时间无效")
	}
	subscriptionSettingsMu.Lock()
	subscriptionSettingsCur = next
	subscriptionSettingsMu.Unlock()
	return saveSubscriptionSettings()
}

// ExitNodeLimitSettings 是全局出口节点限额：对所有国家统一按“国家”计算。
// 运营商只作为节点信息展示，不单独产生配额，因此同一国家无论运营商如何不同，
// 都共享同一个全局国家上限。
type ExitNodeLimitSettings struct {
	Enabled bool `json:"enabled"`
	Limit   int  `json:"limit"`
	// Mode 保留用于兼容旧配置，但不再参与限额计算。
	Mode string `json:"mode,omitempty"` // random / isp；isp 表示同国尽量/必须使用不同运营商
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

func loadExitNodeLimitSettings(dir string) (ExitNodeLimitSettings, error) {
	exitNodeLimitPath = exitNodeLimitFilePath(dir)
	s := ExitNodeLimitSettings{Enabled: true, Limit: 5, Mode: "isp"}
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
	if strings.ToLower(strings.TrimSpace(s.Mode)) != "random" && strings.ToLower(strings.TrimSpace(s.Mode)) != "isp" {
		s.Mode = "isp"
	}
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
