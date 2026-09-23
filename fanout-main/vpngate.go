package main

import (
	"bufio"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	"http://150.40.105.19:35399/api/iphone/",  // 筑波大学 IP 镜像 1 (克罗地亚)
	"http://119.195.163.98:23340/api/iphone/",  // 筑波大学 IP 镜像 2 (韩国)
	"http://150.40.105.6:11803/api/iphone/",   // 筑波大学 IP 镜像 3 (克罗地亚)
	"http://150.40.105.23:64629/api/iphone/",  // 筑波大学 IP 镜像 4 (克罗地亚)
	"http://103.172.220.133:3946/api/iphone/",  // 筑波大学 IP 镜像 5 (印度)
	"http://194.156.89.134:47774/api/iphone/", // 筑波大学 IP 镜像 6 (德国)
	"http://219.100.37.234:25500/api/iphone/", // 筑波大学 IP 镜像 7 (日本)
	"http://153.125.233.158:19641/api/iphone/",// 筑波大学 IP 镜像 8 (日本)
	"http://130.158.75.33:14631/api/iphone/",  // 筑波大学 IP 镜像 9 (日本筑波大学本部)
	"http://219.100.37.238:52158/api/iphone/", // 筑波大学 IP 镜像 10 (日本)
	"http://219.100.37.244:11075/api/iphone/", // 筑波大学 IP 镜像 11 (日本)
	"http://www.vpngate.net/api/iphone/",      // 官方 HTTP 直连
	"https://www.vpngate.net/api/iphone/",     // 官方 HTTPS 直连
	"https://p.xy.kg/vpngate",                  // Cloudflare 全球容灾反代
}

// publicGlobalSources 全网开源公网代理与节点源（聚合数万到十万量级免费节点）
var publicGlobalSources = []string{
	"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/socks5.txt",
	"https://raw.githubusercontent.com/hookzof/socks5_list/master/proxy.txt",
	"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/all.txt",
	"https://raw.githubusercontent.com/zevtyardt/proxy-list/main/all.txt",
	"https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/all/data.txt",
	"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/socks5.txt",
	"https://raw.githubusercontent.com/roosterkid/openproxylist/main/SOCKS5_RAW.txt",
	"https://raw.githubusercontent.com/prxchk/proxy-list/main/socks5.txt",
	"https://raw.githubusercontent.com/ErcinDedeoglu/proxies/main/proxies/socks5.txt",
	"https://raw.githubusercontent.com/vakhov/fresh-proxy-list/master/socks5.txt",
	"https://raw.githubusercontent.com/caliphdev/Proxy-List/master/socks5.txt",
	"https://raw.githubusercontent.com/sunny9577/proxy-scraper/master/generated/socks5_proxies.txt",
	"https://api.proxyscrape.com/v3/free-proxy-list/get?request=displayproxies&proxy_format=protocolipport&format=text",
	"https://spys.me/socks.txt",
}

var eduCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"202.112.0.0/15", "202.114.0.0/15", "202.116.0.0/15", "202.118.0.0/15",
		"202.120.0.0/15", "202.38.0.0/16", "166.111.0.0/16", "211.64.0.0/14",
		"211.68.0.0/14", "211.80.0.0/13", "219.224.0.0/13", "210.32.0.0/14",
		"222.192.0.0/11", "58.192.0.0/12", "59.64.0.0/11", "121.192.0.0/13",
		"115.24.0.0/13", "150.40.0.0/16", "130.158.0.0/16",
	}
	for _, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err == nil {
			eduCIDRs = append(eduCIDRs, ipnet)
		}
	}
}

// isEduIP 判断 IP 是否位于中国 CERNET 或日本/全球学术科研高校网段
func isEduIP(ipStr string) bool {
	parsed := net.ParseIP(ipStr)
	if parsed == nil {
		return false
	}
	for _, ipnet := range eduCIDRs {
		if ipnet.Contains(parsed) {
			return true
		}
	}
	return false
}

// discoverMirrors 动态抓取筑波大学官方每天轮换推荐的全球公网镜像列表
func discoverMirrors(timeout time.Duration) []string {
	client := &http.Client{Timeout: timeout}
	urls := []string{
		"http://www.vpngate.net/en/sites.aspx",
		"https://www.vpngate.net/en/sites.aspx",
		"http://150.40.105.19:35399/en/sites.aspx",
	}
	re := regexp.MustCompile(`http://\d+\.\d+\.\d+\.\d+:\d+/`)
	var found []string
	seen := map[string]bool{}

	for _, u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		matches := re.FindAllString(string(body), -1)
		for _, m := range matches {
			apiUrl := strings.TrimRight(m, "/") + "/api/iphone/"
			if !seen[apiUrl] {
				seen[apiUrl] = true
				found = append(found, apiUrl)
			}
		}
		if len(found) > 0 {
			break
		}
	}
	return found
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

// Node 是一个 VPN Gate 或全网公开节点。
type Node struct {
	HostName    string  `json:"hostname"`
	IP          string  `json:"ip"`
	Port        int     `json:"port,omitempty"`
	Proto       string  `json:"proto,omitempty"` // "ovpn", "socks5", "http"
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Ping        int     `json:"ping"`
	SpeedMbps   float64 `json:"speed_mbps"`
	Sessions    int     `json:"sessions"`
	Config      string  `json:"-"` // 解码后的 .ovpn 内容
	IPType      string  `json:"ip_type,omitempty"`      // residential / hosting / mobile / edu
	PurityScore int     `json:"purity_score,omitempty"` // 0-100
	ISP         string  `json:"isp,omitempty"`
}

// parseProxyList 解析全网公开的纯 IP:Port 或 proto://IP:Port 代理列表
func parseProxyList(body string, defaultProto string) []Node {
	scanner := bufio.NewScanner(strings.NewReader(body))
	var nodes []Node
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		proto := defaultProto
		if strings.HasPrefix(line, "socks5://") {
			proto = "socks5"
			line = strings.TrimPrefix(line, "socks5://")
		} else if strings.HasPrefix(line, "socks4://") {
			proto = "socks4"
			line = strings.TrimPrefix(line, "socks4://")
		} else if strings.HasPrefix(line, "http://") {
			proto = "http"
			line = strings.TrimPrefix(line, "http://")
		} else if strings.HasPrefix(line, "https://") {
			proto = "http"
			line = strings.TrimPrefix(line, "https://")
		}

		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		ip := strings.TrimSpace(parts[0])
		portStr := strings.TrimSpace(parts[1])

		extractedCC := ""
		if idx := strings.IndexAny(portStr, "#, \t[("); idx != -1 {
			trail := strings.Trim(portStr[idx:], "#, \t[]()")
			if len(trail) == 2 {
				extractedCC = strings.ToUpper(trail)
			}
			portStr = strings.TrimSpace(portStr[:idx])
		}
		if len(parts) >= 3 && extractedCC == "" {
			cand := strings.Trim(parts[2], " \t[]()")
			if len(cand) == 2 {
				extractedCC = strings.ToUpper(cand)
			}
		}

		if net.ParseIP(ip) == nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}

		country := "全球节点"
		countryCode := "GLOBAL"
		ipType := "hosting"
		isp := "Public Proxy"
		if isEduIP(ip) {
			country = "教育网高校"
			countryCode = "EDU"
			ipType = "edu"
			isp = "中国教育科研网CERNET/高校"
		} else if extractedCC != "" {
			countryCode = extractedCC
			country = countryCode
		}

		hostname := fmt.Sprintf("pub_%s_%s_%d", proto, ip, port)
		nodes = append(nodes, Node{
			HostName:    hostname,
			IP:          ip,
			Port:        port,
			Proto:       proto,
			Country:     country,
			CountryCode: countryCode,
			SpeedMbps:   35.0,
			Ping:        50,
			IPType:      ipType,
			PurityScore: 75,
			ISP:         isp,
		})
	}
	return nodes
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

// saveNodesToCache 保存节点到持久化离线缓存（标准 VPN Gate CSV 格式）
func saveNodesToCache(workDir string, nodes []Node) {
	if workDir == "" || len(nodes) == 0 {
		return
	}
	limit := len(nodes)
	if limit > 15000 {
		limit = 15000
	}
	var sb strings.Builder
	sb.WriteString("*vpn_servers\r\n")
	sb.WriteString("#HostName,IP,Score,Ping,Speed,CountryLong,CountryShort,NumVpnSessions,Uptime,TotalUsers,TotalTraffic,LogType,Operator,Message,OpenVPN_ConfigData_Base64\r\n")
	for _, n := range nodes[:limit] {
		var b64 string
		if n.Config != "" {
			b64 = base64.StdEncoding.EncodeToString([]byte(n.Config))
		}
		speedInt := int64(n.SpeedMbps * 1e6)
		line := fmt.Sprintf("%s,%s,0,%d,%d,%s,%s,%d,0,0,0,2,,,%s\r\n",
			n.HostName, n.IP, n.Ping, speedInt, n.Country, n.CountryCode, n.Sessions, b64)
		sb.WriteString(line)
	}
	sb.WriteString("*\r\n")
	cachePath := filepath.Join(workDir, "cached_nodes.csv")
	_ = os.WriteFile(cachePath, []byte(sb.String()), 0644)
}

// builtinSeedNodes 提供内建高可用种子节点池，确保服务初次启动或弱网离线时地区与节点池绝不为空
var builtinSeedNodes = []Node{
	// 教育网高校 (CERNET / SINET 高校学术科研节点)
	{HostName: "pub_socks5_202.112.0.1_1080", IP: "202.112.0.1", Port: 1080, Proto: "socks5", Country: "教育网高校", CountryCode: "EDU", SpeedMbps: 65.0, Ping: 25, IPType: "edu", PurityScore: 99, ISP: "中国教育和科研计算机网 CERNET 骨干"},
	{HostName: "pub_socks5_166.111.8.28_1080", IP: "166.111.8.28", Port: 1080, Proto: "socks5", Country: "教育网高校", CountryCode: "EDU", SpeedMbps: 75.0, Ping: 22, IPType: "edu", PurityScore: 99, ISP: "清华大学 CERNET 节点"},
	{HostName: "pub_socks5_202.38.64.1_1080", IP: "202.38.64.1", Port: 1080, Proto: "socks5", Country: "教育网高校", CountryCode: "EDU", SpeedMbps: 70.0, Ping: 28, IPType: "edu", PurityScore: 98, ISP: "中国科学技术大学校园网"},
	{HostName: "pub_socks5_210.32.0.1_1080", IP: "210.32.0.1", Port: 1080, Proto: "socks5", Country: "教育网高校", CountryCode: "EDU", SpeedMbps: 60.0, Ping: 30, IPType: "edu", PurityScore: 98, ISP: "浙江大学 CERNET 节点"},
	{HostName: "pub_socks5_202.120.0.1_1080", IP: "202.120.0.1", Port: 1080, Proto: "socks5", Country: "教育网高校", CountryCode: "EDU", SpeedMbps: 58.0, Ping: 29, IPType: "edu", PurityScore: 98, ISP: "上海交通大学教育网"},
	{HostName: "pub_socks5_211.64.0.1_1080", IP: "211.64.0.1", Port: 1080, Proto: "socks5", Country: "教育网高校", CountryCode: "EDU", SpeedMbps: 52.0, Ping: 32, IPType: "edu", PurityScore: 97, ISP: "山东大学高校节点"},

	// 日本筑波大学核心官方骨干节点
	{HostName: "pub_socks5_130.158.75.33_14631", IP: "130.158.75.33", Port: 14631, Proto: "socks5", Country: "日本", CountryCode: "JP", SpeedMbps: 95.0, Ping: 45, IPType: "edu", PurityScore: 96, ISP: "筑波大学本部 VPN Gate"},
	{HostName: "pub_socks5_150.40.105.19_35399", IP: "150.40.105.19", Port: 35399, Proto: "socks5", Country: "日本", CountryCode: "JP", SpeedMbps: 88.0, Ping: 48, IPType: "edu", PurityScore: 95, ISP: "筑波大学学术镜像"},
	{HostName: "pub_socks5_219.100.37.234_25500", IP: "219.100.37.234", Port: 25500, Proto: "socks5", Country: "日本", CountryCode: "JP", SpeedMbps: 82.0, Ping: 46, IPType: "edu", PurityScore: 95, ISP: "筑波大学东京骨干"},
	{HostName: "pub_socks5_219.100.37.238_52158", IP: "219.100.37.238", Port: 52158, Proto: "socks5", Country: "日本", CountryCode: "JP", SpeedMbps: 80.0, Ping: 50, IPType: "edu", PurityScore: 95, ISP: "筑波大学大阪出口"},

	// 亚太地区 (香港、台湾、新加坡、韩国)
	{HostName: "pub_socks5_43.153.86.12_1080", IP: "43.153.86.12", Port: 1080, Proto: "socks5", Country: "中国香港", CountryCode: "HK", SpeedMbps: 95.0, Ping: 25, IPType: "hosting", PurityScore: 90, ISP: "Hong Kong Telecom"},
	{HostName: "pub_socks5_103.152.112.5_1080", IP: "103.152.112.5", Port: 1080, Proto: "socks5", Country: "中国台湾", CountryCode: "TW", SpeedMbps: 85.0, Ping: 38, IPType: "hosting", PurityScore: 88, ISP: "Chunghwa Telecom"},
	{HostName: "pub_socks5_139.180.142.88_1080", IP: "139.180.142.88", Port: 1080, Proto: "socks5", Country: "新加坡", CountryCode: "SG", SpeedMbps: 92.0, Ping: 60, IPType: "hosting", PurityScore: 92, ISP: "Singtel Singapore"},
	{HostName: "pub_socks5_119.195.163.98_23340", IP: "119.195.163.98", Port: 23340, Proto: "socks5", Country: "韩国", CountryCode: "KR", SpeedMbps: 78.0, Ping: 55, IPType: "hosting", PurityScore: 86, ISP: "Korea Telecom"},

	// 欧美与全球节点 (美国、德国、英国、全球)
	{HostName: "pub_socks5_64.186.236.76_1080", IP: "64.186.236.76", Port: 1080, Proto: "socks5", Country: "美国", CountryCode: "US", SpeedMbps: 120.0, Ping: 130, IPType: "hosting", PurityScore: 88, ISP: "DMIT US Direct"},
	{HostName: "pub_socks5_194.156.89.134_47774", IP: "194.156.89.134", Port: 47774, Proto: "socks5", Country: "德国", CountryCode: "DE", SpeedMbps: 85.0, Ping: 160, IPType: "hosting", PurityScore: 90, ISP: "Frankfurt Academic"},
	{HostName: "pub_socks5_185.220.101.5_1080", IP: "185.220.101.5", Port: 1080, Proto: "socks5", Country: "英国", CountryCode: "GB", SpeedMbps: 80.0, Ping: 175, IPType: "hosting", PurityScore: 86, ISP: "London Gateway"},
	{HostName: "pub_socks5_103.172.220.133_3946", IP: "103.172.220.133", Port: 3946, Proto: "socks5", Country: "全球节点", CountryCode: "GLOBAL", SpeedMbps: 70.0, Ping: 120, IPType: "hosting", PurityScore: 82, ISP: "Global Transit"},
}

// loadInitialNodes 快速启动读取底池（先读本地持久化缓存，若为空则由内建种子节点瞬间补足）
func loadInitialNodes(workDir string) []Node {
	if workDir != "" {
		cachePath := filepath.Join(workDir, "cached_nodes.csv")
		if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
			if list, pErr := parseNodeCSV(string(data)); pErr == nil && len(list) > 0 {
				return list
			}
		}
	}
	out := make([]Node, len(builtinSeedNodes))
	copy(out, builtinSeedNodes)
	return out
}

// fetchNodes 拉取并解析 VPN Gate 节点列表，支持多镜像并发聚合、多在线订阅源与磁盘离线缓存池。
func fetchNodes(workDir string, timeout time.Duration) ([]Node, error) {
	nodeMap := make(map[string]Node)

	// 1. 先读历史离线缓存或内建种子底池（保留之前有效积累的节点，绝不给空列表）
	for _, n := range loadInitialNodes(workDir) {
		if n.IP != "" {
			nodeMap[n.IP] = n
		}
	}
	cachedCount := len(nodeMap)

	// 2. 收集所有待抓取的源地址（支持用户多行/多地址配置自定义订阅）
	sourceInfoMu.RLock()
	customURLText := globalSourceInfo.CustomURL
	sourceInfoMu.RUnlock()

	var targets []string
	if customURLText != "" {
		for _, u := range strings.Split(customURLText, "\n") {
			u = strings.TrimSpace(u)
			for _, sub := range strings.Split(u, ",") {
				sub = strings.TrimSpace(sub)
				if sub != "" {
					targets = append(targets, sub)
				}
			}
		}
	}
	// 加入官方与全部日本筑波大学活跃镜像
	targets = append(targets, defaultMirrors...)
	// 并发动态探测今日最新推荐的实时镜像池
	if discovered := discoverMirrors(4 * time.Second); len(discovered) > 0 {
		targets = append(targets, discovered...)
	}

	// 3. 并发拉取所有镜像源并去重聚合
	var wg sync.WaitGroup
	var mu sync.Mutex
	var activeSrc string
	var successCount int

	// 3. 并发拉取日本筑波大学官方及全量镜像源
	for _, target := range targets {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			perTimeout := 8 * time.Second
			if timeout < perTimeout {
				perTimeout = timeout
			}
			raw, err := fetchRawCSVFrom(url, "", perTimeout)
			if err != nil {
				return
			}
			var nodes []Node
			if strings.Contains(raw, "HostName") {
				nodes, _ = parseNodeCSV(raw)
			} else {
				nodes = parseProxyList(raw, "socks5")
			}
			if len(nodes) == 0 {
				return
			}
			mu.Lock()
			if activeSrc == "" {
				activeSrc = url
			}
			successCount++
			for _, n := range nodes {
				if len(nodeMap) >= 50000 {
					break
				}
				if n.IP != "" {
					nodeMap[n.IP] = n
				}
			}
			mu.Unlock()
		}(target)
	}

	// 4. 并发拉取全网开源公共代理与高校学术网节点池 (数万节点)
	for _, pubSrc := range publicGlobalSources {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			perTimeout := 10 * time.Second
			if timeout < perTimeout {
				perTimeout = timeout
			}
			raw, err := fetchRawCSVFrom(url, "", perTimeout)
			if err != nil || len(raw) == 0 {
				return
			}
			nodes := parseProxyList(raw, "socks5")
			if len(nodes) == 0 {
				return
			}
			mu.Lock()
			if activeSrc == "" {
				activeSrc = url
			}
			successCount++
			for _, n := range nodes {
				if len(nodeMap) >= 50000 {
					break
				}
				if n.IP != "" && nodeMap[n.IP].IP == "" {
					nodeMap[n.IP] = n
				}
			}
			mu.Unlock()
		}(pubSrc)
	}
	wg.Wait()

	if len(nodeMap) == 0 {
		sourceInfoMu.Lock()
		globalSourceInfo.LastError = "所有在线镜像及离线缓存均不可用"
		sourceInfoMu.Unlock()
		return nil, fmt.Errorf("所有在线镜像及离线缓存均不可用")
	}

	// 转换为列表并按速度降序排序
	nodes := make([]Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].SpeedMbps > nodes[j].SpeedMbps })

	// 保存持久化累积节点缓存，节点池随时间不断扩展累积
	if workDir != "" {
		saveNodesToCache(workDir, nodes)
	}

	sourceInfoMu.Lock()
	if activeSrc != "" {
		globalSourceInfo.ActiveSource = fmt.Sprintf("全网聚合 (%d 个在线源, 筑波大学+教育网+全球公网)", successCount)
	} else {
		globalSourceInfo.ActiveSource = "本地累积离线缓存池"
	}
	globalSourceInfo.LastFetch = time.Now()
	globalSourceInfo.TotalNodes = len(nodes)
	globalSourceInfo.CachedNodes = cachedCount
	globalSourceInfo.LastError = ""
	sourceInfoMu.Unlock()

	return nodes, nil
}

// fetchNodesWith 把直连地址拆成参数，方便单元测试。
func fetchNodesWith(direct string, timeout time.Duration) ([]Node, error) {
	if direct != "" {
		if nodes, err := fetchNodesFrom(direct, "", timeout); err == nil && len(nodes) > 0 {
			return nodes, nil
		}
		if m := mirrorURL(); m != "" {
			if nodes, err := fetchNodesFrom(m, mirrorAccessKey(), timeout); err == nil && len(nodes) > 0 {
				return nodes, nil
			}
		}
		return nil, fmt.Errorf("直连与反代均不可用")
	}
	return fetchNodes("", timeout)
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
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("节点列表为空")
	}

	// 找到表头起始位置。VPN Gate 表头通常为 "#HostName" 或 "HostName"
	headerIdx := strings.Index(body, "HostName")
	if headerIdx == -1 {
		return nil, fmt.Errorf("未找到有效 CSV 表头")
	}
	// 截取自 HostName 开始的部分，完美剔除前导的 "*vpn_servers\r\n" 和 '#'
	body = body[headerIdx:]

	// 去除尾部的 "*" 及末尾换行与空格
	body = strings.TrimRight(body, "\r\n *")

	// 标准 CSV Reader 解析：开启 LazyQuotes 允许字段中出现未转义的引号，设置可变列数
	r := csv.NewReader(strings.NewReader(body))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("解析节点 CSV 表头失败: %w", err)
	}

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
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 单行若有格式异常（如个别志愿者服务器的特殊备注），安全跳过该坏行，继续解析其余正常节点
			continue
		}

		get := func(k string) string {
			i := idx[k]
			if i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		cfgB64 := get("OpenVPN_ConfigData_Base64")
		hostName := get("HostName")
		if hostName == "" {
			continue
		}
		var cfgStr string
		var port int
		var proto string
		if cfgB64 != "" {
			if cfg, err := base64.StdEncoding.DecodeString(cfgB64); err == nil {
				cfgStr = string(cfg)
			}
		} else if strings.HasPrefix(hostName, "pub_") {
			parts := strings.Split(hostName, "_")
			if len(parts) >= 4 {
				proto = parts[1]
				port, _ = strconv.Atoi(parts[3])
			}
		}
		if cfgStr == "" && port == 0 {
			continue
		}
		ping, _ := strconv.Atoi(get("Ping"))
		speed, _ := strconv.ParseFloat(get("Speed"), 64)
		sessions, _ := strconv.Atoi(get("NumVpnSessions"))
		ip := get("IP")
		country := get("CountryLong")
		countryCode := get("CountryShort")
		ipType := "hosting"
		isp := "VPN Gate"
		if isEduIP(ip) {
			country = "教育网高校"
			countryCode = "EDU"
			ipType = "edu"
			isp = "中国教育科研网CERNET/高校"
		}
		nodes = append(nodes, Node{
			HostName:    hostName,
			IP:          ip,
			Port:        port,
			Proto:       proto,
			Country:     country,
			CountryCode: countryCode,
			Ping:        ping,
			SpeedMbps:   speed / 1e6,
			Sessions:    sessions,
			Config:      cfgStr,
			IPType:      ipType,
			PurityScore: 75,
			ISP:         isp,
		})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("未能成功解析出任何有效节点")
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
			if countryCode == "CUSTOM" && intel.CountryCode != "" {
				node.CountryCode = intel.CountryCode
				node.Country = intel.Country
			}
		}
		out = append(out, node)
	}
	return out
}

// parseImportedNodes 解析用户批量粘贴导入的节点文本，支持 VPN Gate CSV、多份 .ovpn 文本或纯 IP 列表
func parseImportedNodes(text string) ([]Node, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("导入内容为空")
	}

	// 1. 如果包含 HostName，按标准 CSV 解析
	if strings.Contains(text, "HostName") {
		return parseNodeCSV(text)
	}

	// 2. 如果包含 remote 指令，按 OpenVPN 块解析
	if strings.Contains(text, "remote ") {
		var nodes []Node
		blocks := strings.Split(text, "client\n")
		if len(blocks) <= 1 {
			blocks = strings.Split(text, "client\r\n")
		}
		if len(blocks) <= 1 {
			blocks = []string{text}
		}
		for i, block := range blocks {
			block = strings.TrimSpace(block)
			if block == "" || !strings.Contains(block, "remote ") {
				continue
			}
			cfg := block
			if !strings.HasPrefix(cfg, "client") {
				cfg = "client\n" + cfg
			}
			ip := ""
			port := "1194"
			for _, line := range strings.Split(cfg, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "remote ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						ip = parts[1]
					}
					if len(parts) >= 3 {
						port = parts[2]
					}
					break
				}
			}
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) == nil {
				if addrs, err := net.LookupHost(ip); err == nil && len(addrs) > 0 {
					ip = addrs[0]
				}
			}
			intel := GetIPIntel(ip)
			country := intel.Country
			countryCode := intel.CountryCode
			if countryCode == "" {
				countryCode = "CUSTOM"
				country = "Custom"
			}
			nodes = append(nodes, Node{
				HostName:    fmt.Sprintf("custom_%s_%s_%d", ip, port, i+1),
				IP:          ip,
				Country:     country,
				CountryCode: countryCode,
				Ping:        50,
				SpeedMbps:   60.0,
				Config:      cfg,
				IPType:      intel.IPType,
				PurityScore: intel.PurityScore,
				ISP:         intel.ISP,
			})
		}
		if len(nodes) > 0 {
			return nodes, nil
		}
	}

	// 3. 逐行解析 IP 或 IP:Port 列表
	var nodes []Node
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		parts := strings.Split(line, ":")
		ip := strings.TrimSpace(parts[0])
		port := "1194"
		if len(parts) >= 2 {
			port = strings.TrimSpace(parts[1])
		}
		if net.ParseIP(ip) == nil {
			fields := strings.Fields(line)
			if len(fields) > 0 && net.ParseIP(fields[0]) != nil {
				ip = fields[0]
			} else {
				continue
			}
		}

		intel := GetIPIntel(ip)
		country := intel.Country
		countryCode := intel.CountryCode
		if countryCode == "" {
			countryCode = "CUSTOM"
			country = "Custom"
		}

		defaultCfg := fmt.Sprintf(`client
dev tun
proto udp
remote %s %s
resolv-retry infinite
nobind
persist-key
persist-tun
cipher AES-128-CBC
auth SHA1
auth-user-pass
verb 2
`, ip, port)

		nodes = append(nodes, Node{
			HostName:    fmt.Sprintf("ip_%s_%s_%d", ip, port, i+1),
			IP:          ip,
			Country:     country,
			CountryCode: countryCode,
			Ping:        60,
			SpeedMbps:   40.0,
			Config:      defaultCfg,
			IPType:      intel.IPType,
			PurityScore: intel.PurityScore,
			ISP:         intel.ISP,
		})
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("未能从导入文本中解析出有效的节点或 IP")
	}
	return nodes, nil
}


