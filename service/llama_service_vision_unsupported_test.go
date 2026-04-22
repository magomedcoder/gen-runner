//go:build llama

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/magomedcoder/gen-runner/domain"
)

func TestSendMessage_VisionAttachmentErrorsWithoutMMProjOrModels(t *testing.T) {
	svc := NewLlamaService("")
	msgs := []*domain.AIChatMessage{
		{
			Role:              domain.AIChatMessageRoleUser,
			Content:           "опиши изображение",
			AttachmentName:    "image.png",
			AttachmentContent: []byte{1, 2, 3},
		},
	}

	_, err := svc.SendMessage(context.Background(), "any-model", msgs, nil, nil)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}

	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "mmproj") && !strings.Contains(low, "модел") && !strings.Contains(low, "yaml") {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
