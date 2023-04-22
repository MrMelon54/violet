package utils

import (
	"strconv"
	"strings"
)

func SplitDomainPort(host string, defaultPort int) (domain string, port int, ok bool) {
	a := strings.SplitN(host, ":", 2)
	switch len(a) {
	case 2:
		domain = a[0]
		p, err := strconv.Atoi(a[1])
		port = p
		ok = err == nil
	case 1:
		domain = a[0]
		port = defaultPort
		ok = true
	}
	return
}

func GetDomainWithoutPort(domain string) (string, bool) {
	a := strings.SplitN(domain, ":", 2)
	if len(a) == 2 {
		return a[0], true
	}
	if len(a) == 0 {
		return "", false
	}
	return a[0], true
}

func ReplaceSubdomainWithWildcard(domain string) (string, bool) {
	a, b := GetBaseDomain(domain)
	return "*." + a, b
}

func GetBaseDomain(domain string) (string, bool) {
	a := strings.SplitN(domain, ".", 2)
	l := len(a)
	if l == 2 {
		return a[1], true
	}
	if l == 1 {
		return a[0], true
	}
	return "", false
}

func GetTopFqdn(domain string) (string, bool) {
	a := strings.Split(domain, ".")
	l := len(a)
	if l >= 2 {
		return strings.Join(a[l-2:], "."), true
	}
	if l == 1 {
		return domain, true
	}
	return "", false
}
