package mcpclient

import (
	"testing"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestServerConfigFingerprintChangesWithCommand(t *testing.T) {
	a := &domain.MCPServer{
		Transport:      "stdio",
		Command:        "npx",
		ArgsJSON:       `["x"]`,
		EnvJSON:        `{}`,
		URL:            "",
		HeadersJSON:    `{}`,
		TimeoutSeconds: 120,
	}

	fp1 := serverConfigFingerprint(a)
	a.Command = "node"
	fp2 := serverConfigFingerprint(a)
	if fp1 == fp2 {
		t.Fatal("fingerprint should change when command changes")
	}
}

func TestToolsListCacheInvalidateServerID(t *testing.T) {
	c := NewToolsListCache()
	key := listCacheKey{
		id: 1,
		fp: "abc",
	}

	c.mu.Lock()
	c.toolEntries[key] = toolsCacheEntry{until: time.Now().Add(time.Hour)}
	c.resEntries[key] = resourcesCacheEntry{until: time.Now().Add(time.Hour)}
	c.promptsEntries[key] = promptsCacheEntry{until: time.Now().Add(time.Hour)}
	c.mu.Unlock()
	c.InvalidateServerID(1)
	c.mu.RLock()
	_, okTools := c.toolEntries[key]
	_, okRes := c.resEntries[key]
	_, okPr := c.promptsEntries[key]
	c.mu.RUnlock()

	if okTools || okRes || okPr {
		t.Fatal("ожидается удаление ключей tools/resources/prompts")
	}
}

func TestNotifyForListChangedHandlers(t *testing.T) {
	t.Cleanup(func() { SetToolsListCacheForNotifications(nil) })

	SetToolsListCacheForNotifications(nil)
	if g := notifyForListChangedHandlers(nil); g != nil {
		t.Fatalf("want nil, got %p", g)
	}

	c := NewToolsListCache()
	SetToolsListCacheForNotifications(c)
	if g := notifyForListChangedHandlers(nil); g != c {
		t.Fatal("expected process default cache")
	}
	if g := notifyForListChangedHandlers(c); g != c {
		t.Fatal("explicit notify must override default")
	}
}
