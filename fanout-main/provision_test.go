package main

import "testing"

func mgrWith(nodes []Node, running ...string) *Manager {
	m := NewManager(20, t_tmpdir)
	m.nodes = nodes
	for i, h := range running {
		m.tunnels[i+1] = &Tunnel{Slot: i + 1, Node: Node{HostName: h}, Status: "up"}
	}
	return m
}

const t_tmpdir = "/tmp"

var sample = []Node{
	{HostName: "jp1", CountryCode: "JP", Country: "Japan", SpeedMbps: 300, Ping: 10, Config: "test"},
	{HostName: "jp2", CountryCode: "JP", Country: "Japan", SpeedMbps: 200, Ping: 20, Config: "test"},
	{HostName: "kr1", CountryCode: "KR", Country: "Korea", SpeedMbps: 150, Ping: 30, Config: "test"},
}

func TestPickNodesSkipsRunning(t *testing.T) {
	m := mgrWith(sample, "jp1")
	got, err := m.pickNodes("JP", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].HostName != "jp2" {
		t.Fatalf("想拿到未占用的 jp2，实际 %+v", got)
	}
}

func TestPickNodesRegionMismatch(t *testing.T) {
	m := mgrWith(sample, "kr1")
	if _, err := m.pickNodes("KR", 1); err == nil {
		t.Fatal("KR 只有一个节点且已占用，应当报错")
	}
}

func TestRegionsExcludesRunning(t *testing.T) {
	m := mgrWith(sample, "jp1")
	for _, r := range m.Regions() {
		if r.Code == "JP" && r.Available != 1 {
			t.Fatalf("JP 应剩 1 个空闲，实际 %d", r.Available)
		}
		if r.Code == "KR" && r.BestSpeed != 150 {
			t.Fatalf("KR 最高速应为 150，实际 %v", r.BestSpeed)
		}
	}
}

func TestJobLifecycle(t *testing.T) {
	var s JobStore
	j := s.New("测试", []string{"a", "b"})
	if v := j.View(); v.Total != 2 || v.Done != 0 || v.Status != "running" {
		t.Fatalf("初始状态不对: %+v", v)
	}

	j.Set(0, "ok", "1.2.3.4")
	j.Set(1, "failed", "连不上")
	j.Finish()

	v := j.View()
	if v.Status != "failed" || v.Done != 2 {
		t.Fatalf("有失败步骤时整体应为 failed: %+v", v)
	}
	if v.Steps[0].Detail != "1.2.3.4" {
		t.Fatalf("步骤详情丢失: %+v", v.Steps[0])
	}

	s.Dismiss(j.ID())
	if len(s.Views()) != 0 {
		t.Fatal("Dismiss 后不应还留着")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("第一行\n第二行"); got != "第一行" {
		t.Fatalf("firstLine = %q", got)
	}
	if got := firstLine("只有一行"); got != "只有一行" {
		t.Fatalf("firstLine = %q", got)
	}
}

func TestOrchestrateSourceAllowed(t *testing.T) {
	vpn := Node{Source: "vpngate", CountryCode: "JP", Config: "client"}
	proxy := Node{Source: "proxy", CountryCode: "US", Proto: "socks5", IP: "1.2.3.4", Port: 1080}
	edu := Node{Source: "edu", CountryCode: "KR", IPType: "edu"}
	if !orchestrateSourceAllowed(vpn, []string{"vpngate"}) {
		t.Fatal("vpngate 节点应允许")
	}
	if orchestrateSourceAllowed(vpn, []string{"proxy"}) {
		t.Fatal("vpngate 节点不应被 proxy 源选中")
	}
	if !orchestrateSourceAllowed(proxy, []string{"proxy"}) {
		t.Fatal("proxy 节点应允许")
	}
	if !orchestrateSourceAllowed(edu, []string{"edu"}) {
		t.Fatal("edu 节点应允许")
	}
}

func TestDefaultAutoOrchestrateOptions(t *testing.T) {
	o := DefaultAutoOrchestrateOptions()
	if o.HotTarget != 3 || o.ColdTarget != 1 || o.MaxStarts != 6 {
		t.Fatalf("默认编排参数异常: %+v", o)
	}
}
