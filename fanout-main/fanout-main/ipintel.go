package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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

func saveIPIntel() {
	globalIPIntel.mu.RLock()
	defer globalIPIntel.mu.RUnlock()

	if globalIPIntel.filePath == "" {
		return
	}
	blob, err := json.MarshalIndent(globalIPIntel.cache, "", "  ")
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
		baseScore = 55
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
		return IPIntel{IP: ip, IPType: "hosting", PurityScore: 50, ISP: "Local"}
	}

	globalIPIntel.mu.RLock()
	item, ok := globalIPIntel.cache[ip]
	globalIPIntel.mu.RUnlock()

	if ok && time.Now().Unix()-item.UpdatedAt < 7*86400 {
		return item
	}

	// 单个 IP 查询
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,isp,org,as,mobile,proxy,hosting,query", ip)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		// 查不到时默认兜底为机房
		fallback := IPIntel{IP: ip, IPType: "hosting", PurityScore: 55, ISP: "Unknown", UpdatedAt: time.Now().Unix()}
		return fallback
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
		fallback := IPIntel{IP: ip, IPType: "hosting", PurityScore: 55, ISP: "Unknown", UpdatedAt: time.Now().Unix()}
		return fallback
	}

	ipType, purity := computePurity(data.Hosting, data.Mobile, data.Proxy)
	ispName := data.ISP
	if ispName == "" {
		ispName = data.Org
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

	go saveIPIntel()
	return result
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

	// 限制最多批量拉取 100 个，避免启动时阻塞
	if len(uncached) > 100 {
		uncached = uncached[:100]
	}

	type batchReq struct {
		Query  string `json:"query"`
		Fields string `json:"fields"`
	}
	reqList := make([]batchReq, len(uncached))
	for i, ip := range uncached {
		reqList[i] = batchReq{Query: ip, Fields: "status,country,countryCode,isp,org,mobile,proxy,hosting,query"}
	}

	reqBody, err := json.Marshal(reqList)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Post("http://ip-api.com/batch", "application/json", bytes.NewReader(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	defer resp.Body.Close()

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

	if err := json.NewDecoder(resp.Body).Decode(&resList); err != nil {
		return
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
		globalIPIntel.cache[item.Query] = IPIntel{
			IP:          item.Query,
			IPType:      ipType,
			PurityScore: purity,
			ISP:         isp,
			Country:     item.Country,
			CountryCode: item.CountryCode,
			UpdatedAt:   time.Now().Unix(),
		}
	}
	globalIPIntel.mu.Unlock()

	saveIPIntel()
}
