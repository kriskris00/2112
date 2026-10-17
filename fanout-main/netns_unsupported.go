//go:build !linux

package main

import (
	"fmt"
	"net"
)

// Permit portable unit tests, but never fall back to the host network.
func dialerInNetns(nsName string) func(string, string) (net.Conn, error) {
	return func(string, string) (net.Conn, error) {
		return nil, fmt.Errorf("network namespaces require Linux: %s", nsName)
	}
}

func forceIPv4Network(network string) string {
	switch network {
	case "tcp":
		return "tcp4"
	case "udp":
		return "udp4"
	}
	return network
}
