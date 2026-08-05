package core

import (
	"fmt"
	"net"
	"strings"
)

var privateCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"fc00::/7",
	"::1/128",
}

// ParseTrustedNetworks 解析 TRUSTED_NETWORKS：private 关键字、CIDR、裸 IP。
func ParseTrustedNetworks(raw []string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	var errs []string
	for _, item := range raw {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if strings.EqualFold(s, "private") {
			for _, cidr := range privateCIDRs {
				if _, n, err := net.ParseCIDR(cidr); err == nil {
					nets = append(nets, n)
				}
			}
			continue
		}
		// 允许无前缀的裸 IP，自动按地址族补全掩码。
		if !strings.Contains(s, "/") {
			ip := net.ParseIP(s)
			if ip == nil {
				errs = append(errs, fmt.Sprintf("%q 不是合法 IP/CIDR", item))
				continue
			}
			if ip.To4() != nil {
				s += "/32"
			} else {
				s += "/128"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%q: %v", item, err))
			continue
		}
		nets = append(nets, n)
	}
	if len(errs) > 0 {
		return nets, fmt.Errorf("TRUSTED_NETWORKS 存在无法解析的条目：%s", strings.Join(errs, "; "))
	}
	return nets, nil
}

func isTrustedIP(ip string, nets []*net.IPNet) bool {
	if ip == "" || len(nets) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func (s *Service) trusted(ip string) bool {
	if s == nil {
		return false
	}
	return isTrustedIP(ip, s.TrustedNets)
}
