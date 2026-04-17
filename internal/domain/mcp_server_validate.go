package domain

import (
	"fmt"
	"strings"
)

func ValidateMCPServerStructure(s *MCPServer) error {
	if s == nil {
		return nil
	}

	tr := strings.ToLower(strings.TrimSpace(s.Transport))
	if tr == "" {
		tr = "stdio"
	}

	switch tr {
	case "stdio", "sse", "streamable":
	default:
		return fmt.Errorf("transport: ожидается stdio, sse или streamable")
	}

	ts := s.TimeoutSeconds
	if ts != 0 && (ts < 1 || ts > 600) {
		return fmt.Errorf("timeout_seconds: укажите от 1 до 600 или 0 (значение по умолчанию)")
	}

	return nil
}
