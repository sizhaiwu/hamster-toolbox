package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type sharingRequestKey struct{}
type sharingRequestInfo struct{ origin, clientIP string }

func overlayIPv4(ip net.IP) bool {
	v := ip.To4()
	return v != nil && v[0] == 100 && v[1] >= 64 && v[1] <= 127
}

func validateSharingNetwork(address string, proxies []string) (string, []string, error) {
	address = strings.TrimSpace(address)
	if address != "" {
		u, e := url.Parse(address)
		if e != nil || len(address) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, "*\\ \t\r\n%") {
			return "", nil, errors.New("外网地址请填写完整 HTTPS 地址，例如 https://files.example.com；不要包含子路径、口令或查询参数")
		}
		if u.Port() != "" {
			port, e := strconv.Atoi(u.Port())
			if e != nil || port < 1 || port > 65535 {
				return "", nil, errors.New("外网地址端口无效")
			}
		}
		address = "https://" + strings.ToLower(u.Host)
		if u.Port() == "443" {
			host := strings.ToLower(u.Hostname())
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			address = "https://" + host
		}
	}
	if len(proxies) > 16 {
		return "", nil, errors.New("最多设置16个可信代理地址")
	}
	clean := []string{}
	for _, entry := range proxies {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		ip := net.ParseIP(entry)
		if ip != nil {
			if ip.IsUnspecified() || ip.IsMulticast() {
				return "", nil, errors.New("可信代理须为具体 IP 或网段，不能填写所有地址")
			}
			entry = ip.String()
		} else {
			_, network, e := net.ParseCIDR(entry)
			if e != nil {
				return "", nil, errors.New("可信代理请填写 IP 或 CIDR 网段，多个用逗号分隔")
			}
			ones, _ := network.Mask.Size()
			if ones == 0 {
				return "", nil, errors.New("不能信任全部来源；请填写实际代理的 IP 或具体网段")
			}
			entry = network.String()
		}
		clean = append(clean, entry)
	}
	return address, clean, nil
}

func isSharingProxy(ip net.IP, proxies []string) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, entry := range proxies {
		if peer := net.ParseIP(entry); peer != nil && peer.Equal(ip) {
			return true
		}
		if _, network, e := net.ParseCIDR(entry); e == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// Only the owner-configured HTTPS origin may replace the backend origin, and
// only through an explicitly trusted proxy. Forwarded headers alone grant no
// access. Unknown proxies keep the direct peer address for login throttling.
func sharingProxyRequest(r *http.Request, cfg sharingConfig, trusted bool) *http.Request {
	if !trusted || cfg.PublicURL == "" {
		return r
	}
	public, _ := url.Parse(cfg.PublicURL)
	if public == nil {
		return r
	}
	secure := r.Header.Get("Origin") == cfg.PublicURL || strings.EqualFold(r.Host, public.Host) || (r.Header.Get("X-Forwarded-Proto") == "https" && strings.EqualFold(r.Header.Get("X-Forwarded-Host"), public.Host))
	if !secure {
		return r
	}
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	client := peer
	// Walk from the proxy nearest this server toward the client. Never accept
	// an arbitrary leftmost value prepended by the requesting client.
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwarded) <= 16 {
		for i := len(forwarded) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(forwarded[i]))
			if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
				client = peer
				break
			}
			client = ip.String()
			if !isSharingProxy(ip, cfg.TrustedProxies) {
				break
			}
		}
	}
	return r.WithContext(context.WithValue(r.Context(), sharingRequestKey{}, sharingRequestInfo{cfg.PublicURL, client}))
}

func browserRequestOrigin(r *http.Request) string {
	if info, ok := r.Context().Value(sharingRequestKey{}).(sharingRequestInfo); ok {
		return info.origin
	}
	scheme := "http://"
	if r.TLS != nil {
		scheme = "https://"
	}
	return scheme + r.Host
}

func browserRequestSecure(r *http.Request) bool {
	return strings.HasPrefix(browserRequestOrigin(r), "https://")
}

func browserClientIP(r *http.Request) string {
	if info, ok := r.Context().Value(sharingRequestKey{}).(sharingRequestInfo); ok {
		return info.clientIP
	}
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	return peer
}
