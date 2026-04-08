package domain

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Runner struct {
	ID            int64
	Name          string
	Host          string
	Port          int32
	Enabled       bool
	SelectedModel string
}

func RunnerListenAddress(host string, port int32) string {
	h := strings.TrimSpace(host)
	if h == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(h, strconv.Itoa(int(port)))
}

func ParseRunnerHostOrHostPort(hostOrHostPort string, fallbackPort int32) (host string, port int32, err error) {
	raw := strings.TrimSpace(hostOrHostPort)
	if raw == "" {
		return "", 0, fmt.Errorf("хост пустой")
	}

	h, pStr, splitErr := net.SplitHostPort(raw)
	if splitErr == nil {
		p64, e := strconv.ParseUint(strings.TrimSpace(pStr), 10, 32)
		if e != nil || p64 == 0 || p64 > 65535 {
			return "", 0, fmt.Errorf("некорректный порт в адресе")
		}

		h = strings.TrimSpace(h)
		if h == "" {
			return "", 0, fmt.Errorf("пустой хост")
		}
		return h, int32(p64), nil
	}

	if fallbackPort <= 0 || fallbackPort > 65535 {
		return "", 0, fmt.Errorf("укажите порт в отдельном поле или в виде host:port")
	}

	h = raw
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") && !strings.Contains(h, "]:") {
		h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
		h = strings.TrimSpace(h)
	}

	if h == "" {
		return "", 0, fmt.Errorf("хост пустой")
	}

	return h, fallbackPort, nil
}
