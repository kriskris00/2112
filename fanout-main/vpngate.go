package main

import (
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const vpngateAPI = "https://www.vpngate.net/api/iphone/"

// vpngateMirror 是直连拿不到节点列表时的兜底（Cloudflare Worker 反代）。
// 用 FANOUT_VPNGATE_MIRROR 可以换成自己的地址，设成空字符串就只走直连。
const vpngateMirror = "https://p.xy.kg/vpngate"

// mirrorKey 只是让反代不被爬虫和端口扫描白嫖，不是安全边界。
const mirrorKey = "8rhIFzFKRJMFAe-xP5OQPclDEvSjKlHo"

// defaultMirrors 日本筑波大学 VPN Gate 官方活跃公网 IP 镜像与直连候选池
var defaultMirrors = []string{
	"http://150.40.105.19:35399/api/iphone/", // 筑波大学 IP 镜像 1 (已实测存活)
	"http://150.40.105.6:11803/api/iphone/",  // 筑波大学 IP 镜像 2
	"http://150.40.105.23:64629/api/iphone/", // 筑波大学 IP 镜像 3
	"http://194.156.89.134:47774/api/iphone/", // 筑波大学 IP 镜像 4
	"http://www.vpngate.net/api/iphone/",     // 官方 HTTP 直连
	"https://www.vpngate.net/api/iphone/",    // 官方 HTTPS 直连
}

// SourceInfo 描述节点源状态
type SourceInfo struct {
	CustomURL        string    `json:"custom_url"`
	ActiveSource     string    `json:"active_source"`
	LastFetch        time.Time `json:"last_fetch"`
	TotalNodes       int       `json:"total_nodes"`
	CustomNodes      int       `json:"custom_nodes"`
	CachedNodes      int       `json:"cached_nodes"`
	AvailableMirrors []string  `json:"available_mirrors"`
	CustomDir        string    `json:"custom_dir"`
	LastError        string    `json:"last_error,omitempty"`
}

var (
	sourceInfoMu     sync.RWMutex
	globalSourceInfo = SourceInfo{
		AvailableMirrors: defaultMirrors,
		ActiveSource:     "自动选择 (筑波大学镜像/官方源)",
	}
)

func GetSourceInfo() SourceInfo {
	sourceInfoMu.RLock()
	defer sourceInfoMu.RUnlock()
	return globalSourceInfo
}

func SetCustomSourceURL(url string) {
	sourceInfoMu.Lock()
	defer sourceInfoMu.Unlock()
	globalSourceInfo.CustomURL = strings.TrimSpace(url)
}

func mirrorURL() string {
	if v, ok := os.LookupEnv("FANOUT_VPNGATE_MIRROR"); ok {
		return strings.TrimSpace(v)
	}
	return vpngateMirror
}

func mirrorAccessKey() string {
	if v, ok := os.LookupEnv("FANOUT_VPNGATE_MIRROR_KEY"); ok {
		return strings.TrimSpace(v)
	}
	return mirrorKey
}

// Node 是一个 VPN Gate 节点。
type Node struct {
	HostName    string  `json:"hostname"`
	IP          string  `json:"ip"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Ping        int     `json:"ping"`
	SpeedMbps   float64 `json:"speed_mbps"`
	Sessions    int     `json:"sessions"`
	Config      string  `json:"-"` // 解码后的 .ovpn 内容
	IPType      string  `json:"ip_type,omitempty"`      // residential / hosting / mobile
	PurityScore int     `json:"purity_score,omitempty"` // 0-100
	ISP         string  `json:"isp,omitempty"`
}

func vpngateAPIURL() string {
	if v, ok := os.LookupEnv("FANOUT_VPNGATE_API"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	sourceInfoMu.RLock()
	custom := globalSourceInfo.CustomURL
	sourceInfoMu.RUnlock()
	if custom != "" {
		return custom
	}
	return vpngateAPI
}

// fetchNodes 拉取并解析 VPN Gate 节点列表，支持多镜像降级与磁盘离线缓存。
func fetchNodes(workDir string, timeout time.Duration) ([]Node, error) {
	nodes, rawBody, srcURL, err := fetchNodesWithMirrors(vpngateAPIURL(), timeout)
	if err == nil && len(nodes) > 0 {
		// 写入本地持久化缓存
		if workDir != "" && rawBody != "" {
			cachePath := filepath.Join(workDir, "cached_nodes.csv")
			_ = os.WriteFile(cachePath, []byte(rawBody), 0644)
		}
		sourceInfoMu.Lock()
		globalSourceInfo.ActiveSource = srcURL
		globalSourceInfo.LastFetch = time.Now()
		globalSourceInfo.LastError = ""
		sourceInfoMu.Unlock()
		return nodes, nil
	}

	// 在线源全部失败时，尝试读取磁盘离线缓存
	if workDir != "" {
		cachePath := filepath.Join(workDir, "cached_nodes.csv")
		if data, readErr := os.ReadFile(cachePath); readErr == nil && len(data) > 0 {
			if cachedNodes, parseErr := parseNodeCSV(string(data)); parseErr == nil && len(cachedNodes) > 0 {
				log.Printf("所有在线节点源拉取受阻，已恢复载入本地离线缓存节点 (%d 个)", len(cachedNodes))
				sourceInfoMu.Lock()
				globalSourceInfo.ActiveSource = "本地离线缓存 (cached_nodes.csv)"
				globalSourceInfo.CachedNodes = len(cachedNodes)
				if err != nil {
					globalSourceInfo.LastError = "在线拉取失败，已使用离线缓存: " + err.Error()
				}
				sourceInfoMu.Unlock()
				return cachedNodes, nil
			}
		}
	}

	sourceInfoMu.Lock()
	if err != nil {
		globalSourceInfo.LastError = err.Error()
	}
	sourceInfoMu.Unlock()
	return nil, err
}

// fetchNodesWith 把直连地址拆成参数，方便单元测试。
func fetchNodesWith(direct string, timeout time.Duration) ([]Node, error) {
	nodes, _, _, err := fetchNodesWithMirrors(direct, timeout)
	return nodes, err
}

// fetchNodesWithMirrors 依次尝试 direct -> 自定义 mirror -> 筑波大学公共镜像池
func fetchNodesWithMirrors(direct string, timeout time.Duration) ([]Node, string, string, error) {
	// 1. 先尝试指定的主直连地址
	raw, err := fetchRawCSVFrom(direct, "", timeout)
	if err == nil {
		nodes, parseErr := parseNodeCSV(raw)
		if parseErr == nil {
			return nodes, raw, direct, nil
		}
		err = parseErr
	}

	// 2. 检查环境变量 FANOUT_VPNGATE_MIRROR 设置
	if envMirror, ok := os.LookupEnv("FANOUT_VPNGATE_MIRROR"); ok {
		if strings.TrimSpace(envMirror) == "" {
			// 测试用例显式设为空，表示不走任何反代镜像
			return nil, "", "", err
		}
		raw, mirrorErr := fetchRawCSVFrom(strings.TrimSpace(envMirror), mirrorAccessKey(), timeout)
		if mirrorErr == nil {
			nodes, parseErr := parseNodeCSV(raw)
			if parseErr == nil {
				return nodes, raw, envMirror, nil
			}
			return nil, "", "", parseErr
		}
		return nil, "", "", fmt.Errorf("直连失败(%v)；反代也失败: %w", err, mirrorErr)
	}

	// 3. 环境变量未设置时，依次轮询内置的筑波大学官方 IP 镜像列表
	var lastErr error = err
	for _, m := range defaultMirrors {
		if m == direct {
			continue
		}
		mirrorTimeout := 8 * time.Second
		if timeout < mirrorTimeout {
			mirrorTimeout = timeout
		}
		mRaw, mErr := fetchRawCSVFrom(m, "", mirrorTimeout)
		if mErr != nil {
			lastErr = mErr
			continue
		}
		nodes, pErr := parseNodeCSV(mRaw)
		if pErr == nil && len(nodes) > 0 {
			log.Printf("成功通过备用镜像源获取节点: %s (共 %d 个节点)", m, len(nodes))
			return nodes, mRaw, m, nil
		}
		lastErr = pErr
	}

	return nil, "", "", fmt.Errorf("直连及所有备用镜像拉取均失败: %w", lastErr)
}

func fetchRawCSVFrom(url, key string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	if key != "" {
		req.Header.Set("X-Fanout-Key", key)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("拉取失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取失败: %w", err)
	}
	return string(raw), nil
}

func fetchNodesFrom(url, key string, timeout time.Duration) ([]Node, error) {
	raw, err := fetchRawCSVFrom(url, key, timeout)
	if err != nil {
		return nil, err
	}
	return parseNodeCSV(raw)
}

// parseNodeCSV 解析 VPN Gate 的 CSV。首行是 "*vpn_servers"，
// 第二行是以 '#' 开头的表头，末行是 "*"。
func parseNodeCSV(body string) ([]Node, error) {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		kept = append(kept, strings.TrimPrefix(line, "#"))
	}
	if len(kept) < 2 {
		return nil, fmt.Errorf("节点列表格式异常: 有效行不足")
	}

	r := csv.NewReader(strings.NewReader(strings.Join(kept, "\n")))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析节点 CSV 失败: %w", err)
	}

	header := records[0]
	idx := map[string]int{}
	for i, name := range header {
		idx[strings.TrimSpace(name)] = i
	}
	need := []string{"HostName", "IP", "CountryLong", "CountryShort", "Ping", "Speed", "OpenVPN_ConfigData_Base64"}
	for _, k := range need {
		if _, ok := idx[k]; !ok {
			return nil, fmt.Errorf("节点列表缺少字段 %s", k)
		}
	}

	var nodes []Node
	for _, rec := range records[1:] {
		get := func(k string) string {
			i := idx[k]
			if i >= len(rec) {
				return ""
			}
			return rec[i]
		}
		cfgB64 := get("OpenVPN_ConfigData_Base64")
		if cfgB64 == "" || get("HostName") == "" {
			continue
		}
		cfg, err := base64.StdEncoding.DecodeString(cfgB64)
		if err != nil {
			continue
		}
		ping, _ := strconv.Atoi(get("Ping"))
		speed, _ := strconv.ParseFloat(get("Speed"), 64)
		sessions, _ := strconv.Atoi(get("NumVpnSessions"))
		nodes = append(nodes, Node{
			HostName:    get("HostName"),
			IP:          get("IP"),
			Country:     get("CountryLong"),
			CountryCode: get("CountryShort"),
			Ping:        ping,
			SpeedMbps:   speed / 1e6,
			Sessions:    sessions,
			Config:      string(cfg),
		})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("节点列表为空")
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].SpeedMbps > nodes[j].SpeedMbps })
	return nodes, nil
}

// loadLocalOvpnNodes 扫描指定目录下的 .ovpn 配置文件并转换为 Node 列表。
// 允许用户放入自定义的任意 OpenVPN 节点配置。
func loadLocalOvpnNodes(dir string) []Node {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Node
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".ovpn") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(raw)
		host := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		ip := ""
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "remote ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					ip = parts[1]
					break
				}
			}
		}
		// 如果 remote 后面是域名，尝试解析为 IP
		if ip != "" && net.ParseIP(ip) == nil {
			if addrs, err := net.LookupHost(ip); err == nil && len(addrs) > 0 {
				ip = addrs[0]
			}
		}

		country := "Custom"
		countryCode := "CUSTOM"
		// 检查文件名是否形如 US_server1.ovpn 或 JP_node.ovpn
		parts := strings.Split(host, "_")
		if len(parts) >= 2 && len(parts[0]) == 2 {
			countryCode = strings.ToUpper(parts[0])
			country = countryCode
		}
		node := Node{
			HostName:    host,
			IP:          ip,
			Country:     country,
			CountryCode: countryCode,
			SpeedMbps:   100.0,
			Config:      content,
		}
		if ip != "" {
			intel := GetIPIntel(ip)
			node.IPType = intel.IPType
			node.PurityScore = intel.PurityScore
			node.ISP = intel.ISP
		}
		out = append(out, node)
	}
	return out
}

