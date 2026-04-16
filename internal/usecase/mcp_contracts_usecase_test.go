package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
)

type fakeMCPRepoContracts struct {
	active []*domain.MCPServer
	err    error
}

func (f fakeMCPRepoContracts) ListGlobal(context.Context) ([]*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPRepoContracts) ListForUser(context.Context, int) ([]*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPRepoContracts) ListActive(context.Context) ([]*domain.MCPServer, error) {
	return f.active, f.err
}

func (f fakeMCPRepoContracts) GetByID(context.Context, int64) (*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPRepoContracts) GetByIDAccessible(context.Context, int64, int) (*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPRepoContracts) GetGlobalByID(context.Context, int64) (*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPRepoContracts) Create(context.Context, *domain.MCPServer) (*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPRepoContracts) UpdateGlobal(context.Context, *domain.MCPServer) error {
	return nil
}

func (f fakeMCPRepoContracts) UpdateOwned(context.Context, *domain.MCPServer, int) error {
	return nil
}

func (f fakeMCPRepoContracts) DeleteGlobal(context.Context, int64) error {
	return nil
}

func (f fakeMCPRepoContracts) DeleteOwned(context.Context, int64, int) error {
	return nil
}
func (f fakeMCPRepoContracts) CountOwnedByUser(context.Context, int) (int64, error) {
	return 0, nil
}

func TestRunActiveContracts(t *testing.T) {
	uc := &MCPContractsUseCase{
		repo: fakeMCPRepoContracts{
			active: []*domain.MCPServer{
				{
					ID:      1,
					Name:    "ok-1",
					Enabled: true,
				},
				{
					ID:      2,
					Name:    "bad-2",
					Enabled: true,
				},
			},
		},
		probeFn: func(_ context.Context, srv *domain.MCPServer, _ *mcpclient.ToolsListCache) (*mcpclient.ServerProbe, error) {
			if srv.ID == 2 {
				return nil, errors.New("probe failed")
			}

			return &mcpclient.ServerProbe{
				ProtocolVersion: "",
			}, nil
		},

		listToolsFn: func(_ context.Context, srv *domain.MCPServer, _ *mcpclient.ToolsListCache) ([]mcpclient.DeclaredTool, error) {
			if srv.ID == 2 {
				return nil, errors.New("list failed")
			}

			return []mcpclient.DeclaredTool{{Name: "echo"}}, nil
		},
	}

	rows, err := uc.RunActiveContracts(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}

	if !rows[0].ContractPassed {
		t.Fatalf("server #1 should pass: %+v", rows[0])
	}

	if rows[1].ContractPassed {
		t.Fatalf("server #2 should fail: %+v", rows[1])
	}
}

func TestRunActiveContractsSummary(t *testing.T) {
	uc := &MCPContractsUseCase{
		repo: fakeMCPRepoContracts{
			active: []*domain.MCPServer{
				{ID: 1, Enabled: true},
			},
		},
		probeFn: func(_ context.Context, _ *domain.MCPServer, _ *mcpclient.ToolsListCache) (*mcpclient.ServerProbe, error) {
			return &mcpclient.ServerProbe{
				ProtocolVersion: "x",
			}, nil
		},
		listToolsFn: func(_ context.Context, _ *domain.MCPServer, _ *mcpclient.ToolsListCache) ([]mcpclient.DeclaredTool, error) {
			return nil, nil
		},
	}

	s, err := uc.RunActiveContractsSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if s != "active_mcp_servers=1 contract_passed=1 contract_failed=0" {
		t.Fatalf("unexpected summary: %q", s)
	}
}
