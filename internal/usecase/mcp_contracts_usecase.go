package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
)

type MCPContractServerResult struct {
	ServerID         int64
	ServerName       string
	ProbeOK          bool
	ToolsListOK      bool
	ProtocolVersion  string
	DeclaredTools    int
	ContractPassed   bool
	ProbeError       string
	ToolsListError   string
	CheckedAtUnixSec int64
}

type MCPContractsUseCase struct {
	repo        domain.MCPServerRepository
	cache       *mcpclient.ToolsListCache
	probeFn     func(context.Context, *domain.MCPServer, *mcpclient.ToolsListCache) (*mcpclient.ServerProbe, error)
	listToolsFn func(context.Context, *domain.MCPServer, *mcpclient.ToolsListCache) ([]mcpclient.DeclaredTool, error)
}

func NewMCPContractsUseCase(repo domain.MCPServerRepository, cache *mcpclient.ToolsListCache) *MCPContractsUseCase {
	return &MCPContractsUseCase{
		repo:  repo,
		cache: cache,
		probeFn: func(ctx context.Context, srv *domain.MCPServer, c *mcpclient.ToolsListCache) (*mcpclient.ServerProbe, error) {
			return mcpclient.ProbeServer(ctx, srv, c)
		},

		listToolsFn: func(ctx context.Context, srv *domain.MCPServer, c *mcpclient.ToolsListCache) ([]mcpclient.DeclaredTool, error) {
			if c == nil {
				return mcpclient.ListTools(ctx, srv)
			}

			return c.ListToolsCached(ctx, srv, mcpclient.DefaultToolsListCacheTTL)
		},
	}
}

func contractCheckTimeout(s *domain.MCPServer) time.Duration {
	sec := int64(30)
	if s != nil && s.TimeoutSeconds > 0 {
		sec = int64(s.TimeoutSeconds)
	}

	if sec < 15 {
		sec = 15
	}

	if sec > 180 {
		sec = 180
	}

	return time.Duration(sec) * time.Second
}

func (u *MCPContractsUseCase) RunActiveContracts(ctx context.Context) ([]MCPContractServerResult, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}

	servers, err := u.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]MCPContractServerResult, 0, len(servers))
	for _, srv := range servers {
		if srv == nil || srv.ID <= 0 || !srv.Enabled {
			continue
		}

		res := MCPContractServerResult{
			ServerID:         srv.ID,
			ServerName:       strings.TrimSpace(srv.Name),
			CheckedAtUnixSec: time.Now().Unix(),
		}

		pctx, cancel := context.WithTimeout(ctx, contractCheckTimeout(srv))
		probe, perr := u.probeFn(pctx, srv, u.cache)
		cancel()
		if perr != nil {
			res.ProbeError = perr.Error()
		} else if probe == nil || strings.TrimSpace(probe.ProtocolVersion) == "" {
			res.ProbeError = "probe contract: empty protocol_version"
		} else {
			res.ProbeOK = true
			res.ProtocolVersion = strings.TrimSpace(probe.ProtocolVersion)
		}

		lctx, cancel := context.WithTimeout(ctx, contractCheckTimeout(srv))
		tools, lerr := u.listToolsFn(lctx, srv, u.cache)
		cancel()
		if lerr != nil {
			res.ToolsListError = lerr.Error()
		} else {
			res.ToolsListOK = true
			res.DeclaredTools = len(tools)
		}

		res.ContractPassed = res.ProbeOK && res.ToolsListOK
		out = append(out, res)
	}

	return out, nil
}

func (u *MCPContractsUseCase) RunActiveContractsSummary(ctx context.Context) (string, error) {
	rows, err := u.RunActiveContracts(ctx)
	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "active_mcp_servers=0", nil
	}

	ok := 0
	for _, r := range rows {
		if r.ContractPassed {
			ok++
		}
	}

	return fmt.Sprintf("active_mcp_servers=%d contract_passed=%d contract_failed=%d", len(rows), ok, len(rows)-ok), nil
}
