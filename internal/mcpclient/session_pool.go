package mcpclient

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	httpReuseSessions  atomic.Bool
	httpPoolMaxIdleSec atomic.Int64
	globalHTTPPool     = &httpSessionPool{byID: make(map[int64]*pooledServerConn)}
)

func init() {
	httpPoolMaxIdleSec.Store(180)
}

func SetHTTPReuseSessions(v bool) {
	httpReuseSessions.Store(v)
}

func SetHTTPSessionMaxIdleSec(sec int) {
	if sec <= 0 {
		sec = 180
	}
	if sec > 600 {
		sec = 600
	}
	httpPoolMaxIdleSec.Store(int64(sec))
}

type pooledServerConn struct {
	mu       sync.Mutex
	fp       string
	session  *mcp.ClientSession
	lastUsed time.Time
}

type httpSessionPool struct {
	mu   sync.Mutex
	byID map[int64]*pooledServerConn
}

func poolSessionFingerprint(ctx context.Context, srv *domain.MCPServer) string {
	base := serverConfigFingerprint(srv)
	runtime := samplingRuntimeFingerprint(ctx)
	if runtime == "" {
		return base
	}

	return base + "|" + runtime
}

func shouldRetryPooledSessionError(parentCtx context.Context, err error) bool {
	return isRetryableTransportError(parentCtx, err)
}

func (p *httpSessionPool) run(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache, fn func(context.Context, *mcp.ClientSession) error) error {
	p.mu.Lock()
	pc, ok := p.byID[srv.ID]
	if !ok {
		pc = &pooledServerConn{}
		p.byID[srv.ID] = pc
	}
	p.mu.Unlock()

	pc.mu.Lock()
	defer pc.mu.Unlock()

	fp := poolSessionFingerprint(ctx, srv)
	maxIdle := time.Duration(httpPoolMaxIdleSec.Load()) * time.Second

	for attempt := 0; attempt < 2; attempt++ {
		now := time.Now()
		if pc.session != nil && (pc.fp != fp || now.Sub(pc.lastUsed) > maxIdle) {
			_ = pc.session.Close()
			pc.session = nil
		}

		reusedSession := pc.session != nil
		opCtx, cancel := context.WithTimeout(ctx, timeoutFor(srv))

		if pc.session == nil {
			transport, err := transportFor(ctx, srv)
			if err != nil {
				cancel()
				return err
			}
			opts := buildMCPClientOptions(ctx, srv, notify)
			cli := mcp.NewClient(&mcp.Implementation{Name: "gen", Version: "1.0.0"}, opts)
			if r := rootsForSession(); len(r) > 0 {
				cli.AddRoots(r...)
			}
			session, err := cli.Connect(opCtx, transport, nil)
			if err != nil {
				cancel()
				return err
			}
			pc.session = session
			pc.fp = fp
		}

		err := fn(opCtx, pc.session)
		cancel()
		pc.lastUsed = time.Now()
		if err == nil {
			return nil
		}

		_ = pc.session.Close()
		pc.session = nil

		if attempt == 0 && reusedSession && shouldRetryPooledSessionError(ctx, err) {
			recordPooledSessionRetry()
			continue
		}

		return err
	}

	return nil
}

func (p *httpSessionPool) closeServer(id int64) {
	if id <= 0 {
		return
	}
	p.mu.Lock()
	pc, ok := p.byID[id]
	if ok {
		delete(p.byID, id)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.session != nil {
		_ = pc.session.Close()
		pc.session = nil
	}
}

func closePooledHTTPSession(serverID int64) {
	globalHTTPPool.closeServer(serverID)
}

func useHTTPSessionPool(ctx context.Context, srv *domain.MCPServer) bool {
	if !httpReuseSessions.Load() || srv == nil || srv.ID <= 0 {
		return false
	}
	tr := strings.ToLower(strings.TrimSpace(srv.Transport))
	if tr != "sse" && tr != "streamable" {
		return false
	}
	return true
}
