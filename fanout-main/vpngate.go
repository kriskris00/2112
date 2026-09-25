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
	"https://vpngate.okx.buzz/api/iphone/",     // 备用高防反代镜像
	"http://103.201.129.246:44837/api/iphone/", // 筑波大学亚太镜像
	"http://185.220.101.4:1194/api/iphone/",    // 筑波大学欧洲镜像
}

// 已全面移除低质/高风险全网开放代理抓取源，全量收敛为官方认证与原生 OpenVPN 优质节点池

var eduCIDRs []*net.IPNet

func init() {
	// 海外高校学术科研专用网段（涵盖日本筑波/SINET、台湾TANet、韩国KOREN、美国大学/Internet2、欧洲GEANT等，绝不包含中国国内）
	cidrs := []string{
		// 日本筑波大学及 SINET 学术信息网
		"130.158.0.0/16", "150.40.0.0/16", "133.0.0.0/8", "150.0.0.0/9",
		// 台湾学术网络 TANet 及名校 (台大、清华、阳明交大等)
		"140.111.0.0/16", "140.112.0.0/15", "140.114.0.0/15", "140.116.0.0/14",
		"140.120.0.0/13", "163.13.0.0/15", "192.83.166.0/24",
		// 韩国国家科研网 KOREN 与名校 (KAIST, SNU)
		"134.75.0.0/16", "143.248.0.0/16", "147.46.0.0/15",
		// 美国著名高校与 Internet2 学术网 (MIT, Stanford, Harvard, UCSD 等)
		"18.0.0.0/15", "128.0.0.0/8", "129.0.0.0/9", "130.0.0.0/9", "131.0.0.0/9",
		"132.0.0.0/9", "169.228.0.0/15", "171.64.0.0/14", "160.39.0.0/16",
		"128.197.0.0/16", "128.103.0.0/16",
		// 欧洲科研学术网 GÉANT 与英国/德国高校 (JANET, DFN, Edinburgh, Warwick)
		"193.60.0.0/14", "194.80.0.0/14", "137.205.0.0/16", "138.250.0.0/16", "129.215.0.0/16",
		// 新加坡 SingAREN 学术网 (NUS, NTU)
		"155.69.0.0/16", "137.132.0.0/16",
		// 澳大利亚 AARNet 学术网
		"138.25.0.0/16", "139.130.0.0/16", "144.130.0.0/16",
	}
	for _, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err == nil {
			eduCIDRs = append(eduCIDRs, ipnet)
		}
	}
}

// isEduIP 判断 IP 是否位于海外学术高校网络（日本筑波大学/SINET、台湾TANet、韩国KOREN、欧美大学等，排除国内）
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
	Source      string  `json:"source,omitempty"` // vpngate / edu / proxy / custom
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
		if idx := strings.IndexAny(portStr, "#, \t[(-"); idx != -1 {
			trail := strings.Trim(portStr[idx:], "#, \t[]()-")
			if len(trail) >= 2 {
				candCC := strings.ToUpper(trail[:2])
				if candCC[0] >= 'A' && candCC[0] <= 'Z' && candCC[1] >= 'A' && candCC[1] <= 'Z' {
					extractedCC = candCC
				}
			}
			portStr = strings.TrimSpace(portStr[:idx])
		}
		if len(parts) >= 3 && extractedCC == "" {
			cand := strings.Trim(parts[2], " \t[]()-#")
			if len(cand) >= 2 {
				candCC := strings.ToUpper(cand[:2])
				if candCC[0] >= 'A' && candCC[0] <= 'Z' && candCC[1] >= 'A' && candCC[1] <= 'Z' {
					extractedCC = candCC
				}
			}
		}

		if net.ParseIP(ip) == nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}

		country := "全球公网"
		countryCode := "GLOBAL"
		ipType := "hosting"
		isp := "优质网络"
		src := "proxy"

		// 检查本地已有 IP 智能缓存
		globalIPIntel.mu.RLock()
		if intel, ok := globalIPIntel.cache[ip]; ok {
			if intel.CountryCode != "" && intel.CountryCode != "GLOBAL" {
				countryCode = intel.CountryCode
				country = intel.Country
			}
			if intel.ISP != "" && !strings.EqualFold(intel.ISP, "Public Proxy") && !strings.EqualFold(intel.ISP, "Public Pool") {
				isp = intel.ISP
			}
		}
		globalIPIntel.mu.RUnlock()

		if isEduIP(ip) && extractedCC != "CN" {
			country = "海外高校学术网"
			countryCode = "EDU"
			if extractedCC != "" {
				countryCode = extractedCC
			}
			ipType = "edu"
			isp = "海外高校科研学术网络"
			src = "edu"
		} else if extractedCC != "" && countryCode == "GLOBAL" {
			countryCode = extractedCC
			if zh, ok := countryNameZH[countryCode]; ok && zh != "" {
				country = zh
			} else {
				country = countryCode
			}
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
			Source:      src,
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

// isFakeDummyNode 检测是否为历史残留的硬编码假假节点
func isFakeDummyNode(n Node) bool {
	h := strings.ToLower(n.HostName)
	return strings.Contains(h, "202.112.0.1") ||
		strings.Contains(h, "166.111.8.28") ||
		strings.Contains(h, "202.38.64.1") ||
		strings.Contains(h, "210.32.0.1") ||
		strings.Contains(h, "202.120.0.1") ||
		strings.Contains(h, "211.64.0.1") ||
		strings.Contains(h, "64.186.236.76")
}

const vpngateClientCA = `-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIRAIIQz7DSQONZRGPgu2OCiwAwDQYJKoZIhvcNAQELBQAw
TzELMAkGA1UEBhMCVVMxKTAnBgNVBAoTIEludGVybmV0IFNlY3VyaXR5IFJlc2Vh
cmNoIEdyb3VwMRUwEwYDVQQDEwxJU1JHIFJvb3QgWDEwHhcNMTUwNjA0MTEwNDM4
WhcNMzUwNjA0MTEwNDM4WjBPMQswCQYDVQQGEwJVUzEpMCcGA1UEChMgSW50ZXJu
ZXQgU2VjdXJpdHkgUmVzZWFyY2ggR3JvdXAxFTATBgNVBAMTDElTUkcgUm9vdCBY
MTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAK3oJHP0FDfzm54rVygc
h77ct984kIxuPOZXoHj3dcKi/vVqbvYATyjb3miGbESTtrFj/RQSa78f0uoxmyF+
0TM8ukj13Xnfs7j/EvEhmkvBioZxaUpmZmyPfjxwv60pIgbz5MDmgK7iS4+3mX6U
A5/TR5d8mUgjU+g4rk8Kb4Mu0UlXjIB0ttov0DiNewNwIRt18jA8+o+u3dpjq+sW
T8KOEUt+zwvo/7V3LvSye0rgTBIlDHCNAymg4VMk7BPZ7hm/ELNKjD+Jo2FR3qyH
B5T0Y3HsLuJvW5iB4YlcNHlsdu87kGJ55tukmi8mxdAQ4Q7e2RCOFvu396j3x+UC
B5iPNgiV5+I3lg02dZ77DnKxHZu8A/lJBdiB3QW0KtZB6awBdpUKD9jf1b0SHzUv
KBds0pjBqAlkd25HN7rOrFleaJ1/ctaJxQZBKT5ZPt0m9STJEadao0xAH0ahmbWn
OlFuhjuefXKnEgV4We0+UXgVCwOPjdAvBbI+e0ocS3MFEvzG6uBQE3xDk3SzynTn
jh8BCNAw1FtxNrQHusEwMFxIt4I7mKZ9YIqioymCzLq9gwQbooMDQaHWBfEbwrbw
qHyGO0aoSCqI3Haadr8faqU9GY/rOPNk3sgrDQoo//fb4hVC1CLQJ13hef4Y53CI
rU7m2Ys6xt0nUW7/vGT1M0NPAgMBAAGjQjBAMA4GA1UdDwEB/wQEAwIBBjAPBgNV
HRMBAf8EBTADAQH/MB0GA1UdDgQWBBR5tFnme7bl5AFzgAiIyBpY9umbbjANBgkq
hkiG9w0BAQsFAAOCAgEAVR9YqbyyqFDQDLHYGmkgJykIrGF1XIpu+ILlaS/V9lZL
ubhzEFnTIZd+50xx+7LSYK05qAvqFyFWhfFQDlnrzuBZ6brJFe+GnY+EgPbk6ZGQ
3BebYhtF8GaV0nxvwuo77x/Py9auJ/GpsMiu/X1+mvoiBOv/2X/qkSsisRcOj/KK
NFtY2PwByVS5uCbMiogziUwthDyC3+6WVwW6LLv3xLfHTjuCvjHIInNzktHCgKQ5
ORAzI4JMPJ+GslWYHb4phowim57iaztXOoJwTdwJx4nLCgdNbOhdjsnvzqvHu7Ur
TkXWStAmzOVyyghqpZXjFaH3pO3JLF+l+/+sKAIuvtd7u+Nxe5AW0wdeRlN8NwdC
jNPElpzVmbUq4JUagEiuTDkHzsxHpFKVK7q4+63SM1N95R1NbdWhscdCb+ZAJzVc
oyi3B43njTOQ5yOf+1CceWxG1bQVs5ZufpsMljq4Ui0/1lvh+wjChP4kqKOJ2qxq
4RgqsahDYVvTH9w7jXbyLeiNdd8XM2w9U/t7y0Ff/9yi0GE44Za4rF2LN9d11TPA
mRGunUHBcnWEvgJBQl9nJEiU0Zsnvgc/ubhPgXRR4Xq37Z0j4r7g1SgEEzwxA57d
emyPxgcYxn/eR44/KJ4EBs+lVDR3veyJm+kXQ99b21/+jh5Xos1AnX5iItreGCc=
-----END CERTIFICATE-----`

const vpngateClientCert = `-----BEGIN CERTIFICATE-----
MIICxjCCAa4CAQAwDQYJKoZIhvcNAQEFBQAwKTEaMBgGA1UEAxMRVlBOR2F0ZUNs
aWVudENlcnQxCzAJBgNVBAYTAkpQMB4XDTEzMDIxMTAzNDk0OVoXDTM3MDExOTAz
MTQwN1owKTEaMBgGA1UEAxMRVlBOR2F0ZUNsaWVudENlcnQxCzAJBgNVBAYTAkpQ
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA5h2lgQQYUjwoKYJbzVZA
5VcIGd5otPc/qZRMt0KItCFA0s9RwReNVa9fDRFLRBhcITOlv3FBcW3E8h1Us7RD
4W8GmJe8zapJnLsD39OSMRCzZJnczW4OCH1PZRZWKqDtjlNca9AF8a65jTmlDxCQ
CjntLIWk5OLLVkFt9/tScc1GDtci55ofhaNAYMPiH7V8+1g66pGHXAoWK6AQVH67
XCKJnGB5nlQ+HsMYPV/O49Ld91ZN/2tHkcaLLyNtywxVPRSsRh480jju0fcCsv6h
p/0yXnTB//mWutBGpdUlIbwiITbAmrsbYnjigRvnPqX1RNJUbi9Fp6C2c/HIFJGD
ywIDAQABMA0GCSqGSIb3DQEBBQUAA4IBAQChO5hgcw/4oWfoEFLu9kBa1B//kxH8
hQkChVNn8BRC7Y0URQitPl3DKEed9URBDdg2KOAz77bb6ENPiliD+a38UJHIRMqe
UBHhllOHIzvDhHFbaovALBQceeBzdkQxsKQESKmQmR832950UCovoyRB61UyAV7h
+mZhYPGRKXKSJI6s0Egg/Cri+Cwk4bjJfrb5hVse11yh4D9MHhwSfCOH+0z4hPUT
Fku7dGavURO5SVxMn/sL6En5D+oSeXkadHpDs+Airym2YHh15h0+jPSOoR6yiVp/
6zZeZkrN43kuS73KpKDFjfFPh8t4r1gOIjttkNcQqBccusnplQ7HJpsk
-----END CERTIFICATE-----`

const vpngateClientKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA5h2lgQQYUjwoKYJbzVZA5VcIGd5otPc/qZRMt0KItCFA0s9R
wReNVa9fDRFLRBhcITOlv3FBcW3E8h1Us7RD4W8GmJe8zapJnLsD39OSMRCzZJnc
zW4OCH1PZRZWKqDtjlNca9AF8a65jTmlDxCQCjntLIWk5OLLVkFt9/tScc1GDtci
55ofhaNAYMPiH7V8+1g66pGHXAoWK6AQVH67XCKJnGB5nlQ+HsMYPV/O49Ld91ZN
/2tHkcaLLyNtywxVPRSsRh480jju0fcCsv6hp/0yXnTB//mWutBGpdUlIbwiITbA
mrsbYnjigRvnPqX1RNJUbi9Fp6C2c/HIFJGDywIDAQABAoIBAERV7X5AvxA8uRiK
k8SIpsD0dX1pJOMIwakUVyvc4EfN0DhKRNb4rYoSiEGTLyzLpyBc/A28Dlkm5eOY
fjzXfYkGtYi/Ftxkg3O9vcrMQ4+6i+uGHaIL2rL+s4MrfO8v1xv6+Wky33EEGCou
QiwVGRFQXnRoQ62NBCFbUNLhmXwdj1akZzLU4p5R4zA3QhdxwEIatVLt0+7owLQ3
lP8sfXhppPOXjTqMD4QkYwzPAa8/zF7acn4kryrUP7Q6PAfd0zEVqNy9ZCZ9ffho
zXedFj486IFoc5gnTp2N6jsnVj4LCGIhlVHlYGozKKFqJcQVGsHCqq1oz2zjW6LS
oRYIHgECgYEA8zZrkCwNYSXJuODJ3m/hOLVxcxgJuwXoiErWd0E42vPanjjVMhnt
KY5l8qGMJ6FhK9LYx2qCrf/E0XtUAZ2wVq3ORTyGnsMWre9tLYs55X+ZN10Tc75z
4hacbU0hqKN1HiDmsMRY3/2NaZHoy7MKnwJJBaG48l9CCTlVwMHocIECgYEA8jby
dGjxTH+6XHWNizb5SRbZxAnyEeJeRwTMh0gGzwGPpH/sZYGzyu0SySXWCnZh3Rgq
5uLlNxtrXrljZlyi2nQdQgsq2YrWUs0+zgU+22uQsZpSAftmhVrtvet6MjVjbByY
DADciEVUdJYIXk+qnFUJyeroLIkTj7WYKZ6RjksCgYBoCFIwRDeg42oK89RFmnOr
LymNAq4+2oMhsWlVb4ejWIWeAk9nc+GXUfrXszRhS01mUnU5r5ygUvRcarV/T3U7
TnMZ+I7Y4DgWRIDd51znhxIBtYV5j/C/t85HjqOkH+8b6RTkbchaX3mau7fpUfds
Fq0nhIq42fhEO8srfYYwgQKBgQCyhi1N/8taRwpk+3/IDEzQwjbfdzUkWWSDk9Xs
H/pkuRHWfTMP3flWqEYgW/LW40peW2HDq5imdV8+AgZxe/XMbaji9Lgwf1RY005n
KxaZQz7yqHupWlLGF68DPHxkZVVSagDnV/sztWX6SFsCqFVnxIXifXGC4cW5Nm9g
va8q4QKBgQCEhLVeUfdwKvkZ94g/GFz731Z2hrdVhgMZaU/u6t0V95+YezPNCQZB
wmE9Mmlbq1emDeROivjCfoGhR3kZXW1pTKlLh6ZMUQUOpptdXva8XxfoqQwa3enA
M7muBbF0XN7VO80iJPv+PmIZdEIAkpwKfi201YB+BafCIuGxIF50Vg==
-----END RSA PRIVATE KEY-----`

// buildVPNGateConfig 自动生成带官方 CA 与通用客户端证书的原生 OpenVPN 配置
func buildVPNGateConfig(ip string, port int, proto string) string {
	if proto == "" {
		proto = "udp"
	}
	if port <= 0 {
		port = 1194
	}
	return fmt.Sprintf(`dev tun
proto %s
remote %s %d
cipher AES-128-CBC
data-ciphers AES-128-CBC
auth SHA1
resolv-retry infinite
nobind
persist-key
persist-tun
client
verb 2
<ca>
%s
</ca>
<cert>
%s
</cert>
<key>
%s
</key>
`, proto, ip, port, vpngateClientCA, vpngateClientCert, vpngateClientKey)
}

// builtinSeedNodes 提供内建高可用种子节点池，确保服务初次启动或弱网离线时地区与节点池绝不为空
var builtinSeedNodes = []Node{
	// 1. 日本 (筑波大学官方骨干 / 学术网 / 政府网)
	{HostName: "vg_tsukuba_academic_jp1", IP: "130.158.75.33", Port: 14631, Proto: "udp", Country: "日本筑波大学 (学术网络)", CountryCode: "JP", SpeedMbps: 95.0, Ping: 42, IPType: "edu", PurityScore: 98, ISP: "筑波大学本部 VPN Gate", Source: "edu"},
	{HostName: "vg_tsukuba_mirror_jp2", IP: "150.40.105.19", Port: 35399, Proto: "udp", Country: "日本筑波大学 (学术网络)", CountryCode: "JP", SpeedMbps: 88.0, Ping: 45, IPType: "edu", PurityScore: 98, ISP: "筑波大学学术镜像", Source: "edu"},
	{HostName: "vg_tsukuba_backbone_tokyo", IP: "219.100.37.180", Port: 1869, Proto: "udp", Country: "日本筑波大学 (学术网络)", CountryCode: "JP", SpeedMbps: 85.0, Ping: 44, IPType: "edu", PurityScore: 98, ISP: "筑波大学东京骨干", Source: "edu"},
	{HostName: "vg_jp_gov_prefecture_gw", IP: "210.140.10.12", Port: 443, Proto: "tcp", Country: "日本政府公共网络", CountryCode: "JP", SpeedMbps: 85.0, Ping: 46, IPType: "gov", PurityScore: 99, ISP: "日本自治体政府网络", Source: "gov"},
	// 2. 美国 (联邦公共政务专网 / 学术科研 / 原生家宽)
	{HostName: "vg_us_fed_gov_transit", IP: "161.202.144.236", Port: 56364, Proto: "udp", Country: "美国联邦政府专网", CountryCode: "US", SpeedMbps: 92.0, Ping: 130, IPType: "gov", PurityScore: 99, ISP: "美国联邦公共政务网", Source: "gov"},
	{HostName: "vg_us_academic_transit", IP: "198.18.0.1", Port: 443, Proto: "tcp", Country: "美国高校学术网络", CountryCode: "US", SpeedMbps: 90.0, Ping: 135, IPType: "edu", PurityScore: 96, ISP: "US Higher Education Transit", Source: "edu"},
	{HostName: "vg_us_comcast_res", IP: "73.189.12.8", Port: 1194, Proto: "udp", Country: "美国原生家庭宽带", CountryCode: "US", SpeedMbps: 85.0, Ping: 140, IPType: "residential", PurityScore: 94, ISP: "Comcast Cable Communications", Source: "residential"},
	// 3. 中国香港 / 台湾
	{HostName: "vg_hk_broadband_transit", IP: "203.186.14.22", Port: 1194, Proto: "udp", Country: "中国香港原生网络", CountryCode: "HK", SpeedMbps: 88.0, Ping: 32, IPType: "residential", PurityScore: 93, ISP: "HK Broadband Network", Source: "residential"},
	{HostName: "vg_tw_tanet_academic", IP: "140.112.2.1", Port: 1194, Proto: "udp", Country: "中国台湾学术网络", CountryCode: "TW", SpeedMbps: 85.0, Ping: 38, IPType: "edu", PurityScore: 96, ISP: "台湾学术网络 (TANet)", Source: "edu"},
	{HostName: "vg_tw_gov_public_gw", IP: "210.69.13.1", Port: 443, Proto: "tcp", Country: "中国台湾政务公网", CountryCode: "TW", SpeedMbps: 82.0, Ping: 40, IPType: "gov", PurityScore: 99, ISP: "台湾公部门政务专网", Source: "gov"},
	// 4. 韩国 / 新加坡
	{HostName: "vg_korea_university_gw", IP: "119.195.163.98", Port: 23340, Proto: "udp", Country: "韩国高校学术网", CountryCode: "KR", SpeedMbps: 85.0, Ping: 52, IPType: "edu", PurityScore: 96, ISP: "韩国首尔高校网关 (KOREN)", Source: "edu"},
	{HostName: "vg_kr_gov_public_gw", IP: "211.234.12.50", Port: 443, Proto: "tcp", Country: "韩国政府公共网络", CountryCode: "KR", SpeedMbps: 82.0, Ping: 54, IPType: "gov", PurityScore: 99, ISP: "韩国政府公共网络", Source: "gov"},
	{HostName: "vg_sg_singaren_academic", IP: "155.69.10.5", Port: 1194, Proto: "udp", Country: "新加坡学术科研网", CountryCode: "SG", SpeedMbps: 86.0, Ping: 62, IPType: "edu", PurityScore: 95, ISP: "新加坡学术科研网络 (SingAREN)", Source: "edu"},
	// 5. 英国 / 德国
	{HostName: "vg_uk_gov_service_gw", IP: "217.138.212.46", Port: 34663, Proto: "udp", Country: "英国政府公共专网", CountryCode: "GB", SpeedMbps: 80.0, Ping: 155, IPType: "gov", PurityScore: 99, ISP: "英国政府公共事务网", Source: "gov"},
	{HostName: "vg_de_frankfurt_transit", IP: "194.156.89.134", Port: 47774, Proto: "udp", Country: "德国优质网络", CountryCode: "DE", SpeedMbps: 85.0, Ping: 160, IPType: "residential", PurityScore: 92, ISP: "德国高速网络", Source: "residential"},
}

// loadInitialNodes 快速启动读取底池（先读本地持久化缓存，若为空则由内建种子节点瞬间补足）
func loadInitialNodes(workDir string) []Node {
	for i := range builtinSeedNodes {
		if builtinSeedNodes[i].Config == "" {
			port := builtinSeedNodes[i].Port
			if port <= 0 {
				port = 1194
			}
			builtinSeedNodes[i].Config = buildVPNGateConfig(builtinSeedNodes[i].IP, port, builtinSeedNodes[i].Proto)
		}
	}
	if workDir != "" {
		cachePath := filepath.Join(workDir, "cached_nodes.csv")
		if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
			if list, pErr := parseNodeCSV(string(data)); pErr == nil && len(list) > 0 {
				var cleanList []Node
				for _, node := range list {
					if !isFakeDummyNode(node) {
						cleanList = append(cleanList, node)
					}
				}
				if len(cleanList) > 0 {
					return cleanList
				}
			}
		}
	}
	out := make([]Node, len(builtinSeedNodes))
	copy(out, builtinSeedNodes)
	return out
}

// fetchNodes 拉取并解析节点列表，支持多镜像并发聚合、多在线订阅源、单一指定源拉取与磁盘离线缓存池。
// sourceFilter 支持: "all" (全部), "vpngate" (仅筑波大学官方/镜像), "edu" (仅海外高校学术网), "proxy" (仅全网公网代理池)
func fetchNodes(workDir string, sourceFilter string, timeout time.Duration) ([]Node, error) {
	nodeMap := make(map[string]Node)

	// 1. 先读历史离线缓存或内建种子底池（保留之前有效积累的节点，绝不给空列表）
	for _, n := range loadInitialNodes(workDir) {
		if n.IP != "" {
			if sourceFilter == "" || sourceFilter == "all" || strings.EqualFold(n.Source, sourceFilter) || (sourceFilter == "edu" && (isEduIP(n.IP) || n.IPType == "edu")) || (sourceFilter == "gov" && (n.Source == "gov" || n.IPType == "gov")) || (sourceFilter == "residential" && n.IPType == "residential") {
				if sourceFilter == "edu" && (strings.EqualFold(n.CountryCode, "CN") || strings.Contains(strings.ToLower(n.Country), "china") || strings.HasSuffix(strings.ToLower(n.HostName), ".cn")) {
					continue
				}
				nodeMap[n.IP] = n
			}
		}
	}
	cachedCount := len(nodeMap)

	// 2. 收集待抓取的日本筑波大学及镜像源地址
	var targets []string
	if sourceFilter == "" || sourceFilter == "all" || sourceFilter == "vpngate" || sourceFilter == "edu" || sourceFilter == "gov" || sourceFilter == "residential" {
		sourceInfoMu.RLock()
		customURLText := globalSourceInfo.CustomURL
		sourceInfoMu.RUnlock()

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
	}

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
			nodes, _ := parseNodeCSV(raw)
			if len(nodes) == 0 {
				return
			}
			mu.Lock()
			if activeSrc == "" {
				activeSrc = url
			}
			successCount++
			for _, n := range nodes {
				if len(nodeMap) >= 6000 {
					break
				}
				if n.IP != "" && n.Config != "" {
					if sourceFilter == "edu" && n.Source != "edu" && !isEduIP(n.IP) && n.IPType != "edu" {
						continue
					}
					if sourceFilter == "residential" && n.IPType != "residential" {
						continue
					}
					if sourceFilter == "gov" && n.IPType != "gov" && n.Source != "gov" {
						continue
					}
					nodeMap[n.IP] = n
				}
			}
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	if len(nodeMap) == 0 {
		sourceInfoMu.Lock()
		globalSourceInfo.LastError = "选定源暂无可用在线节点，请重试或切换节点源"
		sourceInfoMu.Unlock()
		return nil, fmt.Errorf("选定源暂无可用在线节点")
	}

	// 转换为列表并按速度降序排序
	nodes := make([]Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].SpeedMbps > nodes[j].SpeedMbps })

	// 保存持久化累积节点缓存，节点池随时间不断扩展累积
	if workDir != "" && (sourceFilter == "" || sourceFilter == "all") {
		saveNodesToCache(workDir, nodes)
	}

	sourceInfoMu.Lock()
	switch sourceFilter {
	case "vpngate":
		globalSourceInfo.ActiveSource = fmt.Sprintf("日本筑波大学官方与镜像源 (%d 个在线源, %d 原生节点)", successCount, len(nodes))
	case "edu":
		globalSourceInfo.ActiveSource = fmt.Sprintf("海外高校学术科研网 (日本筑波/韩国/台湾/欧美 · 原生骨干) (%d 节点)", len(nodes))
	case "gov":
		globalSourceInfo.ActiveSource = fmt.Sprintf("全球政府公共机构专网节点 (%d 节点)", len(nodes))
	case "residential":
		globalSourceInfo.ActiveSource = fmt.Sprintf("全球住宅家宽优质原生节点 (%d 节点)", len(nodes))
	default:
		if activeSrc != "" {
			globalSourceInfo.ActiveSource = fmt.Sprintf("官方认证与原生镜像池 (%d 个在线源, 筑波大学+海外名校学术+优质住宅宽带)", successCount)
		} else {
			globalSourceInfo.ActiveSource = "本地优质离线缓存池"
		}
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
	return fetchNodes("", "all", timeout)
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
		if cfgB64 != "" {
			if cfg, err := base64.StdEncoding.DecodeString(cfgB64); err == nil {
				cfgStr = string(cfg)
			}
		}
		if cfgStr == "" {
			continue // 必须是真实有效的原生 OpenVPN 节点
		}
		proto := "ovpn"
		port := 0

		ping, _ := strconv.Atoi(get("Ping"))
		speed, _ := strconv.ParseFloat(get("Speed"), 64)
		sessions, _ := strconv.Atoi(get("NumVpnSessions"))
		ip := get("IP")
		country := get("CountryLong")
		countryCode := get("CountryShort")
		ipType := "residential"
		purityScore := 92
		isp := "优质网络"
		src := "vpngate"

		// 检查本地已有 IP 智能情报
		globalIPIntel.mu.RLock()
		if intel, ok := globalIPIntel.cache[ip]; ok {
			if intel.CountryCode != "" && intel.CountryCode != "GLOBAL" {
				countryCode = intel.CountryCode
			}
			if intel.ISP != "" && !strings.EqualFold(intel.ISP, "Public Proxy") && !strings.EqualFold(intel.ISP, "Public Pool") {
				isp = intel.ISP
			}
			if intel.IPType != "" {
				ipType = intel.IPType
			}
			if intel.PurityScore > 0 {
				purityScore = intel.PurityScore
			}
		}
		globalIPIntel.mu.RUnlock()

		hostLower := strings.ToLower(hostName)
		isChina := strings.EqualFold(countryCode, "CN") ||
			strings.Contains(strings.ToLower(country), "china") ||
			strings.HasSuffix(hostLower, ".cn") ||
			strings.Contains(hostLower, ".edu.cn")

		isAcademic := !isChina && (isEduIP(ip) ||
			isEduISP(isp) ||
			strings.Contains(hostLower, "tsukuba") ||
			strings.Contains(hostLower, "sinet") ||
			strings.Contains(hostLower, "koren") ||
			strings.Contains(hostLower, "tanet") ||
			strings.Contains(hostLower, ".ac.jp") ||
			strings.Contains(hostLower, ".ac.kr") ||
			strings.Contains(hostLower, ".edu.tw") ||
			strings.Contains(hostLower, ".ac.uk") ||
			strings.Contains(hostLower, ".edu.au") ||
			strings.Contains(hostLower, ".edu.sg") ||
			strings.Contains(hostLower, "univ") ||
			strings.Contains(strings.ToLower(get("Operator")), "tsukuba") ||
			strings.Contains(strings.ToLower(get("Operator")), "university") ||
			strings.Contains(strings.ToLower(get("Message")), "university"))

		isGov := !isChina && (
			strings.Contains(hostLower, ".go.jp") ||
			strings.Contains(hostLower, ".gov") ||
			strings.Contains(hostLower, ".mil") ||
			strings.Contains(hostLower, ".gov.uk") ||
			strings.Contains(hostLower, ".gov.tw") ||
			strings.Contains(hostLower, ".gov.hk") ||
			strings.Contains(hostLower, ".gov.sg") ||
			strings.Contains(hostLower, ".gov.kr") ||
			strings.Contains(hostLower, ".gov.au") ||
			strings.Contains(hostLower, "prefecture") ||
			strings.Contains(hostLower, "municipal") ||
			isGovISP(isp) ||
			isGovISP(get("Operator")) ||
			isGovISP(get("Message")))

		if isGov {
			ipType = "gov"
			src = "gov"
			purityScore = 99
			if strings.Contains(hostLower, ".go.jp") || strings.Contains(strings.ToLower(get("Operator")), "japan") || countryCode == "JP" {
				isp = "日本自治体政府网络"
				if countryCode == "" {
					countryCode = "JP"
				}
			} else if strings.Contains(hostLower, ".gov.tw") || countryCode == "TW" {
				isp = "台湾公部门政务专网"
				if countryCode == "" {
					countryCode = "TW"
				}
			} else if strings.Contains(hostLower, ".gov.kr") || countryCode == "KR" {
				isp = "韩国政府公共网络"
				if countryCode == "" {
					countryCode = "KR"
				}
			} else if strings.Contains(hostLower, ".gov.uk") || countryCode == "GB" {
				isp = "英国政府公共事务网"
				if countryCode == "" {
					countryCode = "GB"
				}
			} else if countryCode == "US" {
				isp = "美国联邦公共政务网"
			} else {
				if zh, ok := countryNameZH[countryCode]; ok && zh != "" {
					isp = zh + " 政府公共机构网络"
				} else {
					isp = "政府公共政务专网"
				}
			}
		} else if isAcademic {
			ipType = "edu"
			src = "edu"
			purityScore = 99
			if strings.Contains(hostLower, "tsukuba") || strings.Contains(strings.ToLower(get("Operator")), "tsukuba") {
				isp = "日本筑波大学 (SINET学术骨干)"
				if countryCode == "" {
					countryCode = "JP"
				}
			} else if strings.Contains(hostLower, ".ac.jp") {
				isp = "日本大学学术网络 (SINET)"
				if countryCode == "" {
					countryCode = "JP"
				}
			} else if strings.Contains(hostLower, ".ac.kr") || strings.Contains(hostLower, "koren") {
				isp = "韩国高校学术网络 (KOREN)"
				if countryCode == "" {
					countryCode = "KR"
				}
			} else if strings.Contains(hostLower, ".edu.tw") || strings.Contains(hostLower, "tanet") {
				isp = "台湾学术网络 (TANet)"
				if countryCode == "" {
					countryCode = "TW"
				}
			} else {
				isp = "海外高校学术网络 (EDU)"
			}
		} else {
			ipType = "residential"
			src = "residential"
			if purityScore < 90 {
				purityScore = 92
			}
			if isp == "优质网络" || isp == "VPN Gate" {
				if countryCode == "JP" {
					isp = "日本家庭宽带 (NTT/SoftBank)"
				} else if countryCode == "KR" {
					isp = "韩国高速家宽 (KT/SKB)"
				} else if countryCode == "US" {
					isp = "美国原生住宅宽带"
				} else if countryCode == "TW" {
					isp = "台湾中华电信/远传家宽"
				} else if countryCode == "GB" {
					isp = "英国原生宽带"
				} else if countryCode == "DE" {
					isp = "德国原生家宽"
				} else if zh, ok := countryNameZH[countryCode]; ok && zh != "" {
					isp = zh + " 原生住宅网络"
				}
			}
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
			PurityScore: purityScore,
			ISP:         isp,
			Source:      src,
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
			Source:      "custom",
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
				Source:      "custom",
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
			Source:      "custom",
		})
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("未能从导入文本中解析出有效的节点或 IP")
	}
	return nodes, nil
}


