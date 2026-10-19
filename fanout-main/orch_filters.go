package main

import "strings"

func normalizeOrchIPType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "residential", "isp":
		return "residential"
	case "datacenter", "hosting", "business", "server":
		return "datacenter"
	case "mobile":
		return "mobile"
	case "edu":
		return "edu"
	case "gov":
		return "gov"
	default:
		return "unknown"
	}
}
func validOrchIPType(v string) bool {
	switch v {
	case "residential", "datacenter", "mobile", "edu", "gov", "unknown":
		return true
	}
	return false
}
func orchTypeAllowed(v string, allowed []string) bool {
	typ := normalizeOrchIPType(v)
	for _, item := range allowed {
		if item == typ {
			return true
		}
	}
	return false
}
