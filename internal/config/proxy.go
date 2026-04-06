package config

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

func NormalizeProxy(p Proxy) Proxy {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.Password = strings.TrimSpace(p.Password)
	if p.ID == "" {
		p.ID = StableProxyID(p)
	}
	if p.Name == "" && p.Host != "" && p.Port > 0 {
		p.Name = fmt.Sprintf("%s:%d", p.Host, p.Port)
	}
	return p
}

func StableProxyID(p Proxy) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(p.Type)) + "|" + strings.ToLower(strings.TrimSpace(p.Host)) + "|" + fmt.Sprintf("%d", p.Port) + "|" + strings.TrimSpace(p.Username)))
	return "proxy_" + hex.EncodeToString(sum[:6])
}

func (c *Config) Normalize() {
	if c == nil {
		return
	}
	for i := range c.Accounts {
		c.Accounts[i].Email = strings.TrimSpace(c.Accounts[i].Email)
		c.Accounts[i].Mobile = NormalizeMobileForStorage(c.Accounts[i].Mobile)
		c.Accounts[i].Password = strings.TrimSpace(c.Accounts[i].Password)
		c.Accounts[i].ProxyID = strings.TrimSpace(c.Accounts[i].ProxyID)
	}
	for i := range c.Proxies {
		c.Proxies[i] = NormalizeProxy(c.Proxies[i])
	}
}
