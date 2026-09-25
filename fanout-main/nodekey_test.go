package main

import "testing"

func TestNodeKeySeparatesProtocolAndPort(t *testing.T) {
	a := Node{IP: "1.2.3.4", Port: 443, Proto: "socks5"}
	b := Node{IP: "1.2.3.4", Port: 443, Proto: "http"}
	c := Node{IP: "1.2.3.4", Port: 8443, Proto: "socks5"}
	if nodeKey(a) == nodeKey(b) {
		t.Fatal("不同协议的同 IP:port 节点不应被合并")
	}
	if nodeKey(a) == nodeKey(c) {
		t.Fatal("不同端口的节点不应被合并")
	}
}

func TestNodeKeyOpenVPNProtocolDefault(t *testing.T) {
	a := Node{IP: "1.2.3.4", Port: 1194, Config: "client\n"}
	b := Node{IP: "1.2.3.4", Port: 1194, Proto: "ovpn"}
	if nodeKey(a) != nodeKey(b) {
		t.Fatal("OpenVPN 配置节点与显式 ovpn 节点应使用同一身份键")
	}
}
