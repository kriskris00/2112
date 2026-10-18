package main

import (
	"os"
	"testing"
	"time"
)

func TestVPNBookOfficialFeedLive(t *testing.T) {
	if os.Getenv("FANOUT_TEST_LIVE_SOURCES") != "1" {
		t.Skip("opt-in external official-source test")
	}
	nodes := fetchVPNBookCurrent(10 * time.Second)
	if len(nodes) == 0 {
		t.Fatal("official provider returned no usable profiles; network or website parser requires investigation")
	}
	for _, n := range nodes {
		if n.VPNUser != "vpnbook" || n.VPNPass == "" || n.Config == "" {
			t.Fatal("profile credentials missing")
		}
		if err := validateOpenVPNConfig(n.Config); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("downloaded and validated %d official profiles; no live VPN handshake performed", len(nodes))
}
