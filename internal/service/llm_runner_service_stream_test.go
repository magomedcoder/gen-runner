package service

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
	"github.com/magomedcoder/gen/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufConnSize = 4 << 20

type mockStreamRunner struct {
	llmrunnerpb.UnimplementedLLMRunnerServiceServer
	t *testing.T

	assertRequest func(t *testing.T, req *llmrunnerpb.SendMessageRequest)
	chunks        []*llmrunnerpb.ChatResponse
}

func (m *mockStreamRunner) SendMessage(req *llmrunnerpb.SendMessageRequest, stream grpc.ServerStreamingServer[llmrunnerpb.ChatResponse]) error {
	if m.assertRequest != nil {
		m.assertRequest(m.t, req)
	}

	for _, c := range m.chunks {
		if err := stream.Send(c); err != nil {
			return err
		}
	}

	return nil
}

func TestSendMessageWithRunnerToolAction_bufconn_visionRequestAndStream(t *testing.T) {
	img := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	mime := "image/png"
	name := "shot.png"
	ts := time.Unix(1_700_000_000, 0)

	mock := &mockStreamRunner{
		t: t,
		assertRequest: func(t *testing.T, req *llmrunnerpb.SendMessageRequest) {
			t.Helper()
			msgs := req.GetMessages()
			if len(msgs) != 1 {
				t.Fatalf("messages: %d", len(msgs))
			}
			cm := msgs[0]
			if cm.GetAttachmentMime() != mime {
				t.Fatalf("AttachmentMime: %q", cm.GetAttachmentMime())
			}
			if cm.GetAttachmentName() != name {
				t.Fatalf("AttachmentName: %q", cm.GetAttachmentName())
			}
			if string(cm.GetAttachmentContent()) != string(img) {
				t.Fatalf("AttachmentContent len=%d", len(cm.GetAttachmentContent()))
			}
		},
		chunks: []*llmrunnerpb.ChatResponse{
			{Content: "вижу ", Done: false},
			{Content: "png", Done: false, ToolActionJson: ptrStr(`[]`)},
			{Content: "", Done: true},
		},
	}

	lis := bufconn.Listen(bufConnSize)
	srv := grpc.NewServer()
	llmrunnerpb.RegisterLLMRunnerServiceServer(srv, mock)

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("grpc Serve: %v", err)
		}
	}()
	t.Cleanup(func() { srv.Stop() })

	ctx := context.Background()
	conn, err := grpc.NewClient(
		"passthrough:///buf",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := llmrunnerpb.NewLLMRunnerServiceClient(conn)
	runner := &LLMRunnerService{
		client: client,
		model:  "default",
	}

	domainMsgs := []*domain.Message{
		{
			SessionId:         7,
			Role:              domain.MessageRoleUser,
			Content:           "что на картинке",
			CreatedAt:         ts,
			AttachmentName:    name,
			AttachmentMime:    mime,
			AttachmentContent: img,
		},
	}

	ch, toolFn, err := runner.SendMessageWithRunnerToolAction(ctx, 7, "m", domainMsgs, nil, 30, nil)
	if err != nil {
		t.Fatal(err)
	}

	var got string
	for c := range ch {
		got += c.Content
	}

	if got != "вижу png" {
		t.Fatalf("streamed content: %q", got)
	}

	if toolFn == nil || toolFn() != `[]` {
		t.Fatalf("tool blob: %q", toolFn())
	}
}

func ptrStr(s string) *string {
	return &s
}
