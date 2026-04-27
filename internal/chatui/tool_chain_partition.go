package chatui

import (
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
)

type ToolSegment struct {
	LeadIndex int
	ToolStart int
	ToolEnd   int
}

type ToolChainGroup struct {
	Segments          []ToolSegment
	FinalAssistantIdx *int
}

type PartitionElement struct {
	SingleIndex *int
	Chain       *ToolChainGroup
}

func assistantHasToolCalls(m *domain.Message) bool {
	return m != nil &&
		m.Role == domain.MessageRoleAssistant &&
		strings.TrimSpace(m.ToolCallsJSON) != ""
}
func PartitionMessagesForToolChainUI(msgs []*domain.Message) []PartitionElement {
	n := len(msgs)
	var out []PartitionElement
	i := 0
	for i < n {
		m := msgs[i]
		if m == nil {
			out = append(out, PartitionElement{SingleIndex: intPtr(i)})
			i++
			continue
		}

		if !assistantHasToolCalls(m) {
			out = append(out, PartitionElement{SingleIndex: intPtr(i)})
			i++
			continue
		}

		group, next := consumeToolChain(msgs, i)
		if group == nil {
			out = append(out, PartitionElement{SingleIndex: intPtr(i)})
			i = next
			continue
		}

		out = append(out, PartitionElement{Chain: group})
		i = next
	}

	return out
}

func intPtr(v int) *int {
	return &v
}

func consumeToolChain(msgs []*domain.Message, start int) (group *ToolChainGroup, nextIndex int) {
	n := len(msgs)
	cur := start
	var segs []ToolSegment
	for {
		if cur >= n || msgs[cur] == nil || !assistantHasToolCalls(msgs[cur]) {
			if len(segs) == 0 {
				return nil, start + 1
			}
			return &ToolChainGroup{
				Segments:          segs,
				FinalAssistantIdx: nil,
			}, cur
		}

		te := cur + 1
		for te < n && msgs[te] != nil && msgs[te].Role == domain.MessageRoleTool {
			te++
		}

		if te == cur+1 {
			if len(segs) == 0 {
				return nil, cur + 1
			}
			return &ToolChainGroup{
				Segments:          segs,
				FinalAssistantIdx: nil,
			}, cur
		}

		segs = append(segs, ToolSegment{
			LeadIndex: cur,
			ToolStart: cur + 1,
			ToolEnd:   te - 1,
		})

		if te >= n {
			return &ToolChainGroup{
				Segments:          segs,
				FinalAssistantIdx: nil,
			}, n
		}

		nxt := msgs[te]
		if nxt == nil || nxt.Role != domain.MessageRoleAssistant {
			return &ToolChainGroup{Segments: segs, FinalAssistantIdx: nil}, te
		}

		if assistantHasToolCalls(nxt) {
			cur = te
			continue
		}

		fi := te

		return &ToolChainGroup{
			Segments:          segs,
			FinalAssistantIdx: &fi,
		}, te + 1
	}
}
