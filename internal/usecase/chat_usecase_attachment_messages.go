package usecase

import (
	"fmt"
	"strings"

	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
)

const hydratedAttachmentExcerptRunes = 320

func buildExpandedAttachmentMessage(attachmentName, extractedText, userMessage string) (string, error) {
	fileContent, truncated := document.TruncateExtractedText(extractedText, 0)

	var b strings.Builder
	b.WriteString(documentAttachmentInstruction)
	b.WriteString("\n\n")
	if truncated {
		b.WriteString(documentTruncatedNotice)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("Файл «%s»:\n\n```\n%s\n```", attachmentName, fileContent))
	if userMessage != "" {
		b.WriteString("\n\n---\n\n")
		b.WriteString(userMessage)
	}

	return b.String(), nil
}

func buildCompactHydratedAttachmentMessage(attachmentName, extractedText, userMessage string, includeUserMessage bool) string {
	extracted := strings.TrimSpace(extractedText)
	excerpt := truncateStringRunes(extracted, hydratedAttachmentExcerptRunes)

	var b strings.Builder
	fmt.Fprintf(&b, "[attachment_ref: %s]\n", attachmentName)
	if excerpt != "" {
		b.WriteString("Краткое содержание вложения:\n")
		b.WriteString(excerpt)
		b.WriteString("\n")
	}

	userMessage = strings.TrimSpace(userMessage)
	if includeUserMessage && userMessage != "" {
		b.WriteString("\nСообщение пользователя:\n")
		b.WriteString(userMessage)
	}

	return strings.TrimSpace(b.String())
}

func buildMessageWithFile(attachmentName string, attachmentContent []byte, userMessage string) (string, error) {
	extracted, err := document.ExtractText(attachmentName, attachmentContent)
	if err != nil {
		logger.W("ChatUseCase: извлечение текста из вложения %q: %v", attachmentName, err)
		return "", fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
	}

	return buildExpandedAttachmentMessage(attachmentName, extracted, userMessage)
}

func buildContextBlocksFromFiles(attachmentNames []string, attachmentContents [][]byte, maxRunes int) ([]documentContextBlock, error) {
	if len(attachmentNames) == 0 || len(attachmentContents) == 0 || len(attachmentNames) != len(attachmentContents) {
		return nil, fmt.Errorf("некорректный набор вложений")
	}

	perFileRunes := max(maxRunes/len(attachmentNames), 200)
	out := make([]documentContextBlock, 0, len(attachmentNames))
	for i := range attachmentNames {
		extracted, err := document.ExtractText(attachmentNames[i], attachmentContents[i])
		if err != nil {
			logger.W("ChatUseCase: извлечение текста из вложения %q: %v", attachmentNames[i], err)
			return nil, fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
		}

		out = append(out, buildAttachmentContextBlock(attachmentNames[i], extracted, perFileRunes))
	}

	return out, nil
}

func buildMessageWithFiles(attachmentNames []string, attachmentContents [][]byte, userMessage string) (string, error) {
	if len(attachmentNames) == 0 || len(attachmentContents) == 0 || len(attachmentNames) != len(attachmentContents) {
		return "", fmt.Errorf("некорректный набор вложений")
	}

	if len(attachmentNames) == 1 {
		return buildMessageWithFile(attachmentNames[0], attachmentContents[0], userMessage)
	}

	var b strings.Builder
	b.WriteString("Ниже - содержимое нескольких вложенных файлов. Опирайся на них при ответе.\n\n")
	for i := range attachmentNames {
		extracted, err := document.ExtractText(attachmentNames[i], attachmentContents[i])
		if err != nil {
			logger.W("ChatUseCase: извлечение текста из вложения %q: %v", attachmentNames[i], err)
			return "", fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
		}

		fileContent, truncated := document.TruncateExtractedText(extracted, 0)
		b.WriteString(fmt.Sprintf("### Файл %d: %s\n\n", i+1, attachmentNames[i]))
		if truncated {
			b.WriteString(documentTruncatedNotice)
			b.WriteString("\n\n")
		}

		b.WriteString("```\n")
		b.WriteString(fileContent)
		b.WriteString("\n```\n\n")
	}

	if strings.TrimSpace(userMessage) != "" {
		b.WriteString("---\n\n")
		b.WriteString(userMessage)
	}

	return b.String(), nil
}
