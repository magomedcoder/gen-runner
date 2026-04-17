package mcpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const MaxResourceReadBytes = 4 << 20

type DeclaredResource struct {
	URI         string
	Name        string
	Title       string
	Description string
	MIMEType    string
}

type DeclaredPrompt struct {
	Name          string
	Title         string
	Description   string
	ArgumentsJSON string
}

func ListResources(ctx context.Context, srv *domain.MCPServer) ([]DeclaredResource, error) {
	return listResources(ctx, srv, nil)
}

func listResources(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache) ([]DeclaredResource, error) {
	sid := int64(0)
	snm := ""
	if srv != nil {
		sid = srv.ID
		snm = strings.TrimSpace(srv.Name)
	}

	logger.D("MCP listResources: server_id=%d name=%q старт", sid, snm)

	var out []DeclaredResource
	err := withSession(ctx, srv, notify, func(cctx context.Context, session *mcp.ClientSession) error {
		var cursor string
		for {
			p := &mcp.ListResourcesParams{}
			if cursor != "" {
				p.Cursor = cursor
			}
			res, err := session.ListResources(cctx, p)
			if err != nil {
				return err
			}
			for _, r := range res.Resources {
				if r == nil || strings.TrimSpace(r.URI) == "" {
					continue
				}
				out = append(out, DeclaredResource{
					URI:         r.URI,
					Name:        r.Name,
					Title:       r.Title,
					Description: r.Description,
					MIMEType:    r.MIMEType,
				})
			}
			cursor = strings.TrimSpace(res.NextCursor)
			if cursor == "" {
				break
			}
		}
		return nil
	})
	if err != nil {
		logger.W("MCP listResources: server_id=%d name=%q err=%v", sid, snm, err)
	} else {
		logger.D("MCP listResources: server_id=%d name=%q всего=%d", sid, snm, len(out))
	}

	recordListResources(err)
	return out, err
}

func ListPrompts(ctx context.Context, srv *domain.MCPServer) ([]DeclaredPrompt, error) {
	return listPrompts(ctx, srv, nil)
}

func listPrompts(ctx context.Context, srv *domain.MCPServer, notify *ToolsListCache) ([]DeclaredPrompt, error) {
	sid := int64(0)
	snm := ""
	if srv != nil {
		sid = srv.ID
		snm = strings.TrimSpace(srv.Name)
	}

	logger.D("MCP listPrompts: server_id=%d name=%q старт", sid, snm)

	var out []DeclaredPrompt
	err := withSession(ctx, srv, notify, func(cctx context.Context, session *mcp.ClientSession) error {
		var cursor string
		for {
			p := &mcp.ListPromptsParams{}
			if cursor != "" {
				p.Cursor = cursor
			}
			res, err := session.ListPrompts(cctx, p)
			if err != nil {
				return err
			}
			for _, pr := range res.Prompts {
				if pr == nil || strings.TrimSpace(pr.Name) == "" {
					continue
				}
				argsJSON := "[]"
				if len(pr.Arguments) > 0 {
					b, err := json.Marshal(pr.Arguments)
					if err == nil {
						argsJSON = string(b)
					}
				}
				out = append(out, DeclaredPrompt{
					Name:          pr.Name,
					Title:         pr.Title,
					Description:   pr.Description,
					ArgumentsJSON: argsJSON,
				})
			}
			cursor = strings.TrimSpace(res.NextCursor)
			if cursor == "" {
				break
			}
		}
		return nil
	})

	if err != nil {
		logger.W("MCP listPrompts: server_id=%d name=%q err=%v", sid, snm, err)
	} else {
		logger.D("MCP listPrompts: server_id=%d name=%q всего=%d", sid, snm, len(out))
	}

	recordListPrompts(err)
	return out, err
}

type readResourcePartWire struct {
	MIMEType      string `json:"mimeType,omitempty"`
	Text          string `json:"text,omitempty"`
	BlobBase64    string `json:"blob_base64,omitempty"`
	BlobTruncated bool   `json:"blob_truncated,omitempty"`
	BlobBytes     int    `json:"blob_bytes,omitempty"`
	BlobDropped   bool   `json:"blob_dropped,omitempty"`
}

func ReadResourceJSON(ctx context.Context, srv *domain.MCPServer, uri string, notify *ToolsListCache) (string, error) {
	uri = strings.TrimSpace(uri)
	sid := int64(0)
	if srv != nil {
		sid = srv.ID
	}

	if uri == "" {
		logger.W("MCP readResource: server_id=%d пустой uri", sid)
		recordReadResource(errors.New("пустой uri ресурса"))
		return "", errors.New("пустой uri ресурса")
	}

	logger.D("MCP readResource: server_id=%d uri_len=%d", sid, len(uri))

	type wrap struct {
		URI     string                 `json:"uri"`
		Parts   []readResourcePartWire `json:"parts"`
		Warning string                 `json:"warning,omitempty"`
	}

	var payload wrap
	payload.URI = uri

	err := withSession(ctx, srv, notify, func(cctx context.Context, session *mcp.ClientSession) error {
		res, err := session.ReadResource(cctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			return err
		}

		totalBlob := 0
		for _, c := range res.Contents {
			if c == nil {
				continue
			}
			part := readResourcePartWire{MIMEType: c.MIMEType}
			if c.Text != "" {
				part.Text = c.Text
				payload.Parts = append(payload.Parts, part)
				continue
			}
			if len(c.Blob) == 0 {
				payload.Parts = append(payload.Parts, part)
				continue
			}
			if totalBlob+len(c.Blob) > MaxResourceReadBytes {
				payload.Warning = fmt.Sprintf("суммарный объём blob превысил %d байт; часть binary содержимого опущена", MaxResourceReadBytes)
				part.BlobDropped = true
				part.BlobBytes = len(c.Blob)
				payload.Parts = append(payload.Parts, part)
				break
			}
			totalBlob += len(c.Blob)
			b64 := base64.StdEncoding.EncodeToString(c.Blob)
			const maxB64 = 8 << 20
			if len(b64) > maxB64 {
				part.BlobTruncated = true
				part.BlobBase64 = b64[:maxB64]
				part.BlobBytes = len(c.Blob)
				payload.Warning = "base64 обрезан в ответе (слишком велик для контекста)"
			} else {
				part.BlobBase64 = b64
				part.BlobBytes = len(c.Blob)
			}
			payload.Parts = append(payload.Parts, part)
		}
		return nil
	})

	if err != nil {
		logger.W("MCP readResource: server_id=%d uri_len=%d err=%v", sid, len(uri), err)
		recordReadResource(err)
		return "", err
	}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		logger.W("MCP readResource: server_id=%d marshal err=%v", sid, err)
		recordReadResource(err)
		return "", err
	}

	logger.D("MCP readResource: server_id=%d parts=%d json_bytes=%d warning=%q", sid, len(payload.Parts), len(b), strings.TrimSpace(payload.Warning))
	recordReadResource(nil)
	return string(b), nil
}

func GetPromptText(ctx context.Context, srv *domain.MCPServer, name string, arguments map[string]string, notify *ToolsListCache) (string, error) {
	name = strings.TrimSpace(name)
	sid := int64(0)
	if srv != nil {
		sid = srv.ID
	}

	if name == "" {
		logger.W("MCP getPrompt: server_id=%d пустое имя", sid)
		recordGetPrompt(errors.New("пустое имя промпта"))
		return "", errors.New("пустое имя промпта")
	}

	if arguments == nil {
		arguments = map[string]string{}
	}

	logger.D("MCP getPrompt: server_id=%d name=%q args_keys=%d", sid, name, len(arguments))

	var sb strings.Builder
	err := withSession(ctx, srv, notify, func(cctx context.Context, session *mcp.ClientSession) error {
		res, err := session.GetPrompt(cctx, &mcp.GetPromptParams{
			Name:      name,
			Arguments: arguments,
		})
		if err != nil {
			return err
		}
		if d := strings.TrimSpace(res.Description); d != "" {
			fmt.Fprintf(&sb, "Description: %s\n\n", d)
		}
		for i, msg := range res.Messages {
			if msg == nil {
				continue
			}
			fmt.Fprintf(&sb, "--- сообщение %d role=%s ---\n", i+1, msg.Role)
			fmt.Fprintln(&sb, contentToLLMString(msg.Content))
		}
		return nil
	})

	if err != nil {
		logger.W("MCP getPrompt: server_id=%d name=%q err=%v", sid, name, err)
		recordGetPrompt(err)
		return "", err
	}

	out := strings.TrimSpace(sb.String())
	logger.D("MCP getPrompt: server_id=%d name=%q text_runes≈%d", sid, name, utf8.RuneCountInString(out))
	recordGetPrompt(nil)
	return TruncateLLMReply(out, MaxMetaToolReplyRunes), nil
}
