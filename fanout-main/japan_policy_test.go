package main

import "testing"

func TestPickJapanNodesRandomDedupsIP(t *testing.T) {
	m := &Manager{nodes: []Node{
		{HostName: "a", IP: "1.1.1.1", CountryCode: "JP", Config: "x", ISP: "A"},
		{HostName: "b", IP: "1.1.1.1", CountryCode: "JP", Config: "x", ISP: "B"},
		{HostName: "c", IP: "2.2.2.2", CountryCode: "JP", Config: "x", ISP: "C"},
	}}
	got, err := m.pickJapanNodes(2, "random", "all")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n.IP] {
			t.Fatalf("duplicate IP selected: %s", n.IP)
		}
		seen[n.IP] = true
	}
}

func TestPickJapanNodesDifferentISP(t *testing.T) {
	m := &Manager{nodes: []Node{
		{HostName: "a", IP: "1.1.1.1", CountryCode: "JP", Config: "x", ISP: "A"},
		{HostName: "b", IP: "2.2.2.2", CountryCode: "JP", Config: "x", ISP: "A"},
		{HostName: "c", IP: "3.3.3.3", CountryCode: "JP", Config: "x", ISP: "B"},
	}}
	got, err := m.pickJapanNodes(2, "isp", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || normalizeISP(got[0].ISP) == normalizeISP(got[1].ISP) {
		t.Fatalf("expected different ISPs, got %+v", got)
	}
}

func TestPickJapanNodesDifferentISPShortage(t *testing.T) {
	m := &Manager{nodes: []Node{
		{HostName: "a", IP: "1.1.1.1", CountryCode: "JP", Config: "x", ISP: "A"},
		{HostName: "b", IP: "2.2.2.2", CountryCode: "JP", Config: "x", ISP: "A"},
	}}
	if _, err := m.pickJapanNodes(2, "isp", "all"); err == nil {
		t.Fatal("expected different-ISP shortage error")
	}
}
