package config

import "strings"

func (a Account) Identifier() string {
	if strings.TrimSpace(a.Email) != "" {
		return strings.TrimSpace(a.Email)
	}
	if mobile := NormalizeMobileForStorage(a.Mobile); mobile != "" {
		return mobile
	}
	return ""
}

func (a Account) CookieString() string {
	if len(a.Cookie) == 0 {
		return ""
	}
	var parts []string
	for k, v := range a.Cookie {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}
