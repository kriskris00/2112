package main

import (
	"strings"
	"testing"
	"time"
)

func TestExitNodeLimitByCountry(t *testing.T) {
	oldPath := exitNodeLimitPath
	exitNodeLimitPath = t.TempDir() + "/exit_node_limit.json"
	defer func() { exitNodeLimitPath = oldPath }()
	if err := setExitNodeLimitSettings(ExitNodeLimitSettings{Enabled: true, Limit: 2, Mode: "country"}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(100, t.TempDir())
	for i := 0; i < 2; i++ {
		m.tunnels[i+1] = &Tunnel{Slot: i + 1, Status: "up", Node: Node{CountryCode: "US", ISP: "ISP-A", IP: "10.0.0." + string(rune('1'+i))}, TargetRegion: "US", Since: time.Now()}
	}
	err := m.checkExitNodeLimit(Node{CountryCode: "US", ISP: "ISP-B"}, "")
	if err == nil || !strings.Contains(err.Error(), "美国") || !strings.Contains(err.Error(), "达到限额") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExitNodeLimitByISP(t *testing.T) {
	oldPath := exitNodeLimitPath
	exitNodeLimitPath = t.TempDir() + "/exit_node_limit.json"
	defer func() { exitNodeLimitPath = oldPath }()
	if err := setExitNodeLimitSettings(ExitNodeLimitSettings{Enabled: true, Limit: 2, Mode: "isp"}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(100, t.TempDir())
	for i := 0; i < 2; i++ {
		m.tunnels[i+1] = &Tunnel{Slot: i + 1, Status: "up", Node: Node{CountryCode: "JP", ISP: "Carrier-A"}, TargetRegion: "JP", Since: time.Now()}
	}
	if err := m.checkExitNodeLimit(Node{CountryCode: "JP", ISP: "Carrier-B"}, ""); err != nil {
		t.Fatalf("different ISP should still be allowed: %v", err)
	}
	err := m.checkExitNodeLimit(Node{CountryCode: "JP", ISP: "Carrier-A"}, "")
	if err == nil || !strings.Contains(err.Error(), "运营商节点已达到限额") {
		t.Fatalf("unexpected error: %v", err)
	}
}
