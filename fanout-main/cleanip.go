package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// parseCleanIPManualScores accepts scores manually checked on cleanip.io.
// No third-party API is implied. Unknown addresses fail closed.
func parseCleanIPManualScores(raw string) (map[string]int, error) {
	scores := make(map[string]int)
	if len(raw) > 30000 {
		return nil, fmt.Errorf("CleanIP 核验清单过长")
	}
	for i, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.NewReplacer(",", " ", "=", " ", "，", " ").Replace(line))
		if len(fields) != 2 {
			return nil, fmt.Errorf("第 %d 行格式应为 IP 分数", i+1)
		}
		ip := net.ParseIP(fields[0])
		if ip == nil || !ip.IsGlobalUnicast() {
			return nil, fmt.Errorf("第 %d 行 IP 无效", i+1)
		}
		score, err := strconv.Atoi(fields[1])
		if err != nil || score < 0 || score > 100 {
			return nil, fmt.Errorf("第 %d 行分数应为 0–100", i+1)
		}
		scores[ip.String()] = score
	}
	return scores, nil
}

func checkCleanIPScore(ip string, verified map[string]int) (int, error) {
	addr := net.ParseIP(ip)
	if addr == nil {
		return 0, fmt.Errorf("无效的出口 IP")
	}
	score, ok := verified[addr.String()]
	if !ok {
		return 0, fmt.Errorf("IP 未在 CleanIP 手工核验清单中，未自动放行")
	}
	return score, nil
}
