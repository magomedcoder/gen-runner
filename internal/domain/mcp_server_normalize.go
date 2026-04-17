package domain

import "strings"

func NormalizeMCPServer(s *MCPServer) {
	if s == nil {
		return
	}

	s.Name = strings.TrimSpace(s.Name)
	s.Transport = strings.ToLower(strings.TrimSpace(s.Transport))
	if s.Transport == "" {
		s.Transport = "stdio"
	}

	s.Command = strings.TrimSpace(s.Command)
	s.URL = strings.TrimSpace(s.URL)
}
