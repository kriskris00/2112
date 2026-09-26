package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ping0Result 是 Ping0 对“真实出口 IP”的风控判定。
// Ping0 官方 API 的详细 IP 查询需要 API Key；没有 Key 时本程序不会伪称已经完成 Ping0 判定。
type Ping0Result struct {
	IP       string `json:"ip"`
	Location string `json:"location"`
	Country  string `json:"country"`
	ASN      string `json:"asn"`
	ASNName  string `json:"asnname"`
	Org      string `json:"org"`
	IsIDC    bool   `json:"isidc"`
	IPRisk   int    `json:"iprisk"`
	IsNative bool   `json:"isnative"`
	ASNType  string `json:"asntype"`
	OrgType  string `json:"orgtype"`
}

type ping0CacheEntry struct {
	At time.Time
	R  Ping0Result
}

var ping0Store = struct {
	sync.RWMutex
	cache map[string]ping0CacheEntry
}{cache: make(map[string]ping0CacheEntry)}

func ping0APIKey() string { return strings.TrimSpace(os.Getenv("FANOUT_PING0_API_KEY")) }

func ping0MaxRisk() int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("FANOUT_PING0_MAX_RISK")))
	if v <= 0 {
		return 40
	}
	if v > 100 {
		return 100
	}
	return v
}

func ping0Strict() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FANOUT_PING0_STRICT")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// checkPing0 查询真实出口 IP。缓存 24 小时，避免频繁调用付费接口。
func checkPing0(ip string) (Ping0Result, bool, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return Ping0Result{}, false, fmt.Errorf("出口 IP 为空")
	}
	key := ping0APIKey()
	if key == "" {
		if ping0Strict() {
			return Ping0Result{}, false, fmt.Errorf("已启用 Ping0 严格模式，但未配置 FANOUT_PING0_API_KEY")
		}
		return Ping0Result{}, false, nil
	}

	ping0Store.RLock()
	ce, ok := ping0Store.cache[ip]
	ping0Store.RUnlock()
	if ok && time.Since(ce.At) < 24*time.Hour {
		return ce.R, true, nil
	}

	url := "https://ping0.cc/apiloc/apikey(" + key + ")/ip(" + ip + ")"
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Ping0Result{}, false, err
	}
	req.Header.Set("User-Agent", "fanout-ping0-check/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return Ping0Result{}, false, fmt.Errorf("Ping0 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Ping0Result{}, false, fmt.Errorf("Ping0 HTTP %d", resp.StatusCode)
	}
	var r Ping0Result
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Ping0Result{}, false, fmt.Errorf("Ping0 返回格式错误: %w", err)
	}
	if r.IP == "" {
		r.IP = ip
	}
	ping0Store.Lock()
	ping0Store.cache[ip] = ping0CacheEntry{At: time.Now(), R: r}
	ping0Store.Unlock()
	return r, true, nil
}

// enforceCleanExitIP 是最终准入门槛：只检查真实出口 IP，不检查 VPN Gate 中继 IP。
// 默认：Ping0 风控值 <= 40 且非 IDC 且为原生 IP 才允许进入正式出口。
func enforceCleanExitIP(ip string) error {
	r, checked, err := checkPing0(ip)
	if err != nil {
		return err
	}
	if !checked {
		return nil
	}
	if r.IsIDC {
		return fmt.Errorf("Ping0 判定为 IDC 机房 IP（风控 %d）", r.IPRisk)
	}
	if r.IPRisk > ping0MaxRisk() {
		return fmt.Errorf("Ping0 风控值 %d > %d", r.IPRisk, ping0MaxRisk())
	}
	if !r.IsNative {
		return fmt.Errorf("Ping0 判定为非原生 IP（广播 IP）")
	}
	return nil
}

func enrichNodeFromPing0(n *Node, ip string) {
	if n == nil || strings.TrimSpace(ip) == "" {
		return
	}
	r, checked, err := checkPing0(ip)
	if err != nil || !checked {
		return
	}
	n.PurityScore = 100 - r.IPRisk
	if n.PurityScore < 0 {
		n.PurityScore = 0
	}
	if r.IsIDC {
		n.IPType = "hosting"
	} else if r.IsNative && n.IPType == "unknown" {
		n.IPType = "residential"
	}
	if r.Org != "" {
		n.ISP = r.Org
	} else if r.ASNName != "" {
		n.ISP = r.ASNName
	}
}
