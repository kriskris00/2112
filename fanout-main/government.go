package main

import (
	"net/url"
	"strings"
)

// Classification describes evidence from public metadata; it never proves
// institutional ownership, access authorization, or VPN availability.
func governmentClassification(n Node) (status, evidence string) {
	if n.FeedCategory == "gov" {
		u, err := url.Parse(n.AuthorizationURL)
		if err == nil && validFeedURL(n.AuthorizationURL) && governmentDomain(u.Hostname()) {
			return "declared", "管理员声明的授权政府来源，已提供说明链接；未独立核验"
		}
	}
	if n.GovernmentStatus == "suspected" && n.GovernmentEvidence != "" {
		return "suspected", n.GovernmentEvidence
	}
	var reasons []string
	if strings.EqualFold(n.Source, "gov") || strings.EqualFold(n.IPType, "gov") {
		reasons = append(reasons, "公开源或历史缓存带有政府/公共机构标签")
	}
	if isGovISP(n.ISP) {
		reasons = append(reasons, "运营商名称包含政府/公共机构关键词")
	}
	if isGovISP(n.HostName) || governmentDomain(strings.ToLower(strings.TrimSuffix(n.HostName, "."))) {
		reasons = append(reasons, "主机名包含机构关键词或政府域名后缀")
	}
	if len(reasons) > 0 {
		return "suspected", strings.Join(reasons, "；") + "；疑似，未核验"
	}
	return "", ""
}

func annotateGovernment(n *Node) {
	n.GovernmentStatus, n.GovernmentEvidence = governmentClassification(*n)
}

func retainGovernmentEvidence(current, previous Node) Node {
	if nodeKey(current) != nodeKey(previous) {
		return current
	}
	status, _ := governmentClassification(current)
	oldStatus, reason := governmentClassification(previous)
	if status == "" && oldStatus == "suspected" {
		current.GovernmentStatus = "suspected"
		if !strings.HasPrefix(reason, "历史元数据（可能过期）：") {
			reason = "历史元数据（可能过期）：" + reason
		}
		current.GovernmentEvidence = reason
	}
	return current
}
