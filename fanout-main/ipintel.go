package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// isEduISP 判断运营商是否为海外高校或国际教育科研学术网（不含国内）
func isEduISP(isp string) bool {
	low := strings.ToLower(isp)
	if strings.Contains(low, "cernet") || strings.Contains(low, "china") {
		return false
	}
	return strings.Contains(low, "education") ||
		strings.Contains(low, "university") ||
		strings.Contains(low, "college") ||
		strings.Contains(low, "sinet") ||
		strings.Contains(low, "academic") ||
		strings.Contains(low, "campus") ||
		strings.Contains(low, "koren") ||
		strings.Contains(low, "tanet") ||
		strings.Contains(low, "geant") ||
		strings.Contains(low, "tsukuba")
}

// isGovISP 判断运营商是否为政府机构、公共事务或市政网络专网（不含国内）
func isGovISP(isp string) bool {
	low := strings.ToLower(isp)
	if strings.Contains(low, "china") || strings.Contains(low, ".cn") {
		return false
	}
	return strings.Contains(low, "government") ||
		strings.Contains(low, "ministry") ||
		strings.Contains(low, "department of") ||
		strings.Contains(low, "prefecture") ||
		strings.Contains(low, "municipal") ||
		strings.Contains(low, "public safety") ||
		strings.Contains(low, "public sector") ||
		strings.Contains(low, "parliament") ||
		strings.Contains(low, "senate") ||
		strings.Contains(low, "federal") ||
		strings.Contains(low, "state of") ||
		strings.Contains(low, "police") ||
		strings.Contains(low, "customs") ||
		strings.Contains(low, "military") ||
		strings.Contains(low, "national defense") ||
		strings.Contains(low, "lgwan") ||
		strings.Contains(low, "gsn") ||
		strings.Contains(low, "govtech") ||
		strings.Contains(low, "gov.uk") ||
		strings.Contains(low, "gov.sg") ||
		strings.Contains(low, "gov.au") ||
		strings.Contains(low, "bundes") ||
		strings.Contains(low, "stadt") ||
		strings.Contains(low, "city of") ||
		strings.Contains(low, "county of") ||
		strings.Contains(low, ".gov") ||
		strings.Contains(low, ".go.jp")
}

// isHostingISP 判断运营商是否为机房/数据中心/云服务商（纯净度低、被封禁风险高）
func isHostingISP(isp string) bool {
	low := strings.ToLower(isp)
	keywords := []string{
		"amazon", "aws", "google", "microsoft", "azure", "digitalocean", "linode",
		"vultr", "choopa", "hetzner", "ovh", "alibaba", "aliyun", "tencent", "huawei",
		"oracle", "cloudflare", "akamai", "fastly", "leaseweb", "contabo", "cogent",
		"hostinger", "zenlayer", "m247", "ucloud", "baidu", "datacenter", "data center",
		"hosting", "server", "cloud", "vps", "dedic", "colocation", "broadband telecommunication",
		"forcepoint", "zscaler", "quadranet", "psychz", "hostkey", "selectel",
	}
	for _, kw := range keywords {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// IPIntel 存储单个 IP 的归属类型、运营商与纯净度风控数据
type IPIntel struct {
	IP          string `json:"ip"`
	IPType      string `json:"ip_type"`      // "residential" (住宅) | "hosting" (机房/数据中心) | "mobile" (移动)
	PurityScore int    `json:"purity_score"` // 15 - 99
	ISP         string `json:"isp"`          // 运营商名称 (Comcast, AT&T, DigitalOcean 等)
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	UpdatedAt   int64  `json:"updated_at"`
}

type ipIntelStore struct {
	mu       sync.RWMutex
	cache    map[string]IPIntel
	filePath string
}

var globalIPIntel = &ipIntelStore{
	cache: make(map[string]IPIntel),
}

func initIPIntel(workDir string) {
	globalIPIntel.mu.Lock()
	defer globalIPIntel.mu.Unlock()

	globalIPIntel.filePath = filepath.Join(workDir, "ip_intel.json")
	blob, err := os.ReadFile(globalIPIntel.filePath)
	if err == nil {
		_ = json.Unmarshal(blob, &globalIPIntel.cache)
	}
}

var (
	intelDirty     bool
	intelDirtyMu   sync.Mutex
	intelSaveOnce  sync.Once
)

func markIPIntelDirty() {
	intelDirtyMu.Lock()
	intelDirty = true
	intelDirtyMu.Unlock()

	intelSaveOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				intelDirtyMu.Lock()
				dirty := intelDirty
				intelDirty = false
				intelDirtyMu.Unlock()
				if dirty {
					saveIPIntel()
				}
			}
		}()
	})
}

func saveIPIntel() {
	globalIPIntel.mu.RLock()
	defer globalIPIntel.mu.RUnlock()

	if globalIPIntel.filePath == "" {
		return
	}
	blob, err := json.Marshal(globalIPIntel.cache)
	if err != nil {
		return
	}
	tmp := globalIPIntel.filePath + ".tmp"
	if err := os.WriteFile(tmp, blob, 0600); err == nil {
		_ = os.Rename(tmp, globalIPIntel.filePath)
	}
}

// computePurity 根据网络风控指纹计算纯净度与类型
func computePurity(hosting, mobile, proxy bool) (string, int) {
	ipType := "residential"
	baseScore := 94

	if hosting {
		ipType = "hosting"
		baseScore = 35
	} else if mobile {
		ipType = "mobile"
		baseScore = 86
	}

	if proxy {
		baseScore -= 20
	}

	if baseScore > 99 {
		baseScore = 99
	}
	if baseScore < 15 {
		baseScore = 15
	}
	return ipType, baseScore
}

// GetIPIntel 获取指定 IP 的情报（带内存与本地文件缓存）
func GetIPIntel(ip string) IPIntel {
	if ip == "" || ip == "127.0.0.1" {
		return IPIntel{IP: ip, IPType: "residential", PurityScore: 80, ISP: "Local"}
	}
	if isEduIP(ip) {
		return IPIntel{
			IP:          ip,
			IPType:      "edu",
			PurityScore: 98,
			ISP:         "海外高校学术科研网络",
			Country:     "海外学术网络",
			CountryCode: "EDU",
			UpdatedAt:   time.Now().Unix(),
		}
	}

	globalIPIntel.mu.RLock()
	item, ok := globalIPIntel.cache[ip]
	globalIPIntel.mu.RUnlock()

	if ok {
		return item
	}

	// 缓存未命中时返回默认住宅家宽预估值，绝不误杀为 hosting，不阻塞网络请求
	fallback := IPIntel{
		IP:          ip,
		IPType:      "residential",
		PurityScore: 88,
		ISP:         "优质家宽",
		Country:     "全球节点",
		CountryCode: "GLOBAL",
		UpdatedAt:   time.Now().Unix(),
	}

	// 异步由后台排队丰富该单个 IP，不阻塞主流程
	go enrichSingleIPAsync(ip)

	return fallback
}

// enrichSingleIPAsync 后台异步查询单个 IP 情报并写入缓存
func enrichSingleIPAsync(ip string) {
	if ip == "" || ip == "127.0.0.1" || isEduIP(ip) {
		return
	}
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,isp,org,as,mobile,proxy,hosting,query", ip)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	defer resp.Body.Close()

	var data struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		Mobile      bool   `json:"mobile"`
		Proxy       bool   `json:"proxy"`
		Hosting     bool   `json:"hosting"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || data.Status != "success" {
		return
	}

	ipType, purity := computePurity(data.Hosting, data.Mobile, data.Proxy)
	ispName := data.ISP
	if ispName == "" {
		ispName = data.Org
	}
	if isGovISP(ispName) && data.CountryCode != "CN" && !strings.Contains(strings.ToLower(data.Country), "china") {
		ipType = "gov"
		purity = 99
	} else if (isEduISP(ispName) || isEduIP(ip)) && data.CountryCode != "CN" && !strings.Contains(strings.ToLower(data.Country), "china") {
		ipType = "edu"
		purity = 98
		if data.CountryCode == "" || data.CountryCode == "EDU" {
			data.CountryCode = "EDU"
			data.Country = "海外高校学术网络"
		}
	} else if data.Hosting || isHostingISP(ispName) || isHostingISP(data.Org) {
		ipType = "hosting"
		purity = 35
	} else {
		ipType = "residential"
		if purity < 80 {
			purity = 92
		}
	}

	result := IPIntel{
		IP:          ip,
		IPType:      ipType,
		PurityScore: purity,
		ISP:         ispName,
		Country:     data.Country,
		CountryCode: data.CountryCode,
		UpdatedAt:   time.Now().Unix(),
	}

	globalIPIntel.mu.Lock()
	globalIPIntel.cache[ip] = result
	globalIPIntel.mu.Unlock()

	markIPIntelDirty()
}

// ResolveIPIntel 同步或从缓存获取 IP 情报（支持本地海外学术网段、ip-api 与 ipwho.is 双重在线容灾）
func ResolveIPIntel(ip string) IPIntel {
	if ip == "" || ip == "127.0.0.1" {
		return IPIntel{IP: ip, CountryCode: "GLOBAL", Country: "全球公网", ISP: "公网代理"}
	}

	// 1. 先查内存缓存
	globalIPIntel.mu.RLock()
	if item, ok := globalIPIntel.cache[ip]; ok {
		if item.CountryCode != "" && item.CountryCode != "GLOBAL" && item.ISP != "" && !strings.EqualFold(item.ISP, "Public Proxy") {
			globalIPIntel.mu.RUnlock()
			return item
		}
	}
	globalIPIntel.mu.RUnlock()

	// 2. 海外学术高校科研网段检测 (日本筑波大学/SINET、韩国KOREN、台湾TANet、欧美名校等)
	if isEduIP(ip) {
		cCode := "JP"
		cName := "日本"
		isp := "日本筑波大学 (SINET学术骨干)"
		if strings.HasPrefix(ip, "140.11") || strings.HasPrefix(ip, "163.13") || strings.HasPrefix(ip, "192.83") {
			cCode = "TW"
			cName = "中国台湾"
			isp = "台湾学术网络 (TANet)"
		} else if strings.HasPrefix(ip, "134.75") || strings.HasPrefix(ip, "143.248") || strings.HasPrefix(ip, "147.46") {
			cCode = "KR"
			cName = "韩国"
			isp = "韩国高校学术网络 (KOREN)"
		} else if strings.HasPrefix(ip, "18.") || strings.HasPrefix(ip, "128.") || strings.HasPrefix(ip, "169.228") || strings.HasPrefix(ip, "171.64") {
			cCode = "US"
			cName = "美国"
			isp = "美国著名高校 (Internet2)"
		} else if strings.HasPrefix(ip, "155.69") || strings.HasPrefix(ip, "137.132") {
			cCode = "SG"
			cName = "新加坡"
			isp = "新加坡先进科研网 (SingAREN)"
		} else if strings.HasPrefix(ip, "138.25") || strings.HasPrefix(ip, "139.130") {
			cCode = "AU"
			cName = "澳大利亚"
			isp = "澳大利亚学术网络 (AARNet)"
		}
		res := IPIntel{
			IP:          ip,
			IPType:      "edu",
			PurityScore: 99,
			ISP:         isp,
			Country:     cName,
			CountryCode: cCode,
			UpdatedAt:   time.Now().Unix(),
		}
		globalIPIntel.mu.Lock()
		globalIPIntel.cache[ip] = res
		globalIPIntel.mu.Unlock()
		markIPIntelDirty()
		return res
	}

	// 3. 在线接口查询 (首选 ip-api.com，备用 ipwho.is)
	client := &http.Client{Timeout: 1800 * time.Millisecond}

	var cc, country, ispName string
	// 尝试 ip-api.com
	url1 := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,isp,org", ip)
	resp1, err1 := client.Get(url1)
	if err1 == nil && resp1.StatusCode == http.StatusOK {
		var d1 struct {
			Status      string `json:"status"`
			Country     string `json:"country"`
			CountryCode string `json:"countryCode"`
			ISP         string `json:"isp"`
			Org         string `json:"org"`
		}
		if json.NewDecoder(resp1.Body).Decode(&d1) == nil && d1.Status == "success" {
			cc = strings.ToUpper(strings.TrimSpace(d1.CountryCode))
			ispName = strings.TrimSpace(d1.ISP)
			if ispName == "" {
				ispName = strings.TrimSpace(d1.Org)
			}
			country = strings.TrimSpace(d1.Country)
		}
		resp1.Body.Close()
	}

	// 备用：尝试 ipwho.is
	if cc == "" || ispName == "" {
		url2 := fmt.Sprintf("http://ipwho.is/%s", ip)
		resp2, err2 := client.Get(url2)
		if err2 == nil && resp2.StatusCode == http.StatusOK {
			var d2 struct {
				Success     bool   `json:"success"`
				Country     string `json:"country"`
				CountryCode string `json:"country_code"`
				Connection  struct {
					ISP string `json:"isp"`
					Org string `json:"org"`
				} `json:"connection"`
			}
			if json.NewDecoder(resp2.Body).Decode(&d2) == nil && d2.Success {
				if cc == "" {
					cc = strings.ToUpper(strings.TrimSpace(d2.CountryCode))
				}
				if country == "" {
					country = strings.TrimSpace(d2.Country)
				}
				if ispName == "" {
					ispName = strings.TrimSpace(d2.Connection.ISP)
					if ispName == "" {
						ispName = strings.TrimSpace(d2.Connection.Org)
					}
				}
			}
			resp2.Body.Close()
		}
	}

	// 4. 规范化国家中文名
	if zh, ok := countryNameZH[cc]; ok && zh != "" {
		country = zh
	}
	if country == "" {
		country = cc
	}
	if country == "" {
		country = "全球公网"
		cc = "GLOBAL"
	}
	if ispName == "" || strings.EqualFold(ispName, "Public Proxy") || strings.EqualFold(ispName, "Public Pool") {
		ispName = "优质网络"
	}

	ipType := "residential"
	purityScore := 92
	if isGovISP(ispName) {
		ipType = "gov"
		purityScore = 99
	} else if isEduISP(ispName) || isEduIP(ip) {
		ipType = "edu"
		purityScore = 98
	} else if isHostingISP(ispName) {
		ipType = "hosting"
		purityScore = 35
	}

	res := IPIntel{
		IP:          ip,
		IPType:      ipType,
		PurityScore: purityScore,
		ISP:         ispName,
		Country:     country,
		CountryCode: cc,
		UpdatedAt:   time.Now().Unix(),
	}

	globalIPIntel.mu.Lock()
	globalIPIntel.cache[ip] = res
	globalIPIntel.mu.Unlock()
	markIPIntelDirty()
	return res
}

// EnrichNodeWithIntel 根据 IP 情报为节点填充准确的国家、国旗代码与企业/运营商名称
func EnrichNodeWithIntel(node *Node) {
	if node == nil || node.IP == "" {
		return
	}
	intel := ResolveIPIntel(node.IP)
	if intel.CountryCode != "" && intel.CountryCode != "GLOBAL" {
		node.CountryCode = intel.CountryCode
		node.Country = intel.Country
	}
	if intel.ISP != "" && !strings.EqualFold(intel.ISP, "Public Proxy") && !strings.EqualFold(intel.ISP, "Public Pool") {
		node.ISP = intel.ISP
	}
}

// BatchEnrichNodes 异步批量解析节点列表的 IP 纯净度与类型（100 个一批）
func BatchEnrichNodes(nodes []Node) {
	if len(nodes) == 0 {
		return
	}

	// 挑出未缓存的 IP
	var uncached []string
	globalIPIntel.mu.RLock()
	now := time.Now().Unix()
	for _, n := range nodes {
		if n.IP == "" {
			continue
		}
		item, ok := globalIPIntel.cache[n.IP]
		if !ok || now-item.UpdatedAt > 7*86400 {
			uncached = append(uncached, n.IP)
		}
	}
	globalIPIntel.mu.RUnlock()

	if len(uncached) == 0 {
		return
	}

	maxTotal := len(uncached)
	if maxTotal > 500 {
		maxTotal = 500
	}

	for start := 0; start < maxTotal; start += 100 {
		end := start + 100
		if end > maxTotal {
			end = maxTotal
		}
		chunk := uncached[start:end]

		type batchReq struct {
			Query  string `json:"query"`
			Fields string `json:"fields"`
		}
		reqList := make([]batchReq, len(chunk))
		for i, ip := range chunk {
			reqList[i] = batchReq{Query: ip, Fields: "status,country,countryCode,isp,org,mobile,proxy,hosting,query"}
		}

		reqBody, err := json.Marshal(reqList)
		if err != nil {
			continue
		}

		client := &http.Client{Timeout: 6 * time.Second}
		resp, err := client.Post("http://ip-api.com/batch", "application/json", bytes.NewReader(reqBody))
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var resList []struct {
			Status      string `json:"status"`
			Query       string `json:"query"`
			Country     string `json:"country"`
			CountryCode string `json:"countryCode"`
			ISP         string `json:"isp"`
			Org         string `json:"org"`
			Mobile      bool   `json:"mobile"`
			Proxy       bool   `json:"proxy"`
			Hosting     bool   `json:"hosting"`
		}

		decodeErr := json.NewDecoder(resp.Body).Decode(&resList)
		resp.Body.Close()
		if decodeErr != nil {
			continue
		}

		globalIPIntel.mu.Lock()
		for _, item := range resList {
			if item.Status != "success" {
				continue
			}
			ipType, purity := computePurity(item.Hosting, item.Mobile, item.Proxy)
			isp := item.ISP
			if isp == "" {
				isp = item.Org
			}
			country := item.Country
			countryCode := item.CountryCode
			if isGovISP(isp) && item.CountryCode != "CN" && !strings.Contains(strings.ToLower(item.Country), "china") {
				ipType = "gov"
				purity = 99
			} else if (isEduISP(isp) || isEduIP(item.Query)) && item.CountryCode != "CN" && !strings.Contains(strings.ToLower(item.Country), "china") {
				ipType = "edu"
				purity = 98
				if countryCode == "" || countryCode == "EDU" {
					countryCode = "EDU"
					country = "海外高校学术网络"
				}
			}
			globalIPIntel.cache[item.Query] = IPIntel{
				IP:          item.Query,
				IPType:      ipType,
				PurityScore: purity,
				ISP:         isp,
				Country:     country,
				CountryCode: countryCode,
				UpdatedAt:   time.Now().Unix(),
			}
		}
		globalIPIntel.mu.Unlock()

		markIPIntelDirty()
		if end < maxTotal {
			time.Sleep(300 * time.Millisecond)
		}
	}
}
