import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:gen/presentation/widgets/safe_markdown_body.dart';
import 'package:gen/core/redacted_thinking_split.dart';
import 'package:gen/core/docx_file_export.dart';
import 'package:gen/core/chat_image_attachment.dart';
import 'package:gen/core/injector.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/core/log/logs.dart';
import 'package:gen/core/session_file_id_scan.dart';
import 'package:gen/core/user_safe_error.dart';
import 'package:gen/presentation/widgets/app_top_notice.dart';
import 'package:gen/core/spreadsheet_file_export.dart';
import 'package:gen/core/user_file_save.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/domain/entities/rag_document_preview.dart';
import 'package:gen/presentation/screens/chat/widgets/rag_context_preview_banner.dart';
import 'package:gen/domain/usecases/chat/get_session_file_usecase.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/widgets/chat_input_bar.dart';
import 'package:gen/presentation/widgets/code_block_builder.dart';

Color _messageBodyTextColor(ColorScheme cs) {
  return cs.onSurface.withValues(alpha: 0.94);
}

MarkdownStyleSheet assistantBubbleMarkdownSheet(ThemeData theme) {
  final cs = theme.colorScheme;
  final onVar = _messageBodyTextColor(cs);
  final base = MarkdownStyleSheet.fromTheme(theme);
  return base.copyWith(
    p: base.p?.copyWith(color: onVar, fontSize: 15, height: 1.5),
    h1: base.h1?.copyWith(color: onVar, height: 1.25),
    h2: base.h2?.copyWith(color: onVar, height: 1.3),
    h3: base.h3?.copyWith(color: onVar, height: 1.35),
    h4: base.h4?.copyWith(color: onVar),
    h5: base.h5?.copyWith(color: onVar),
    h6: base.h6?.copyWith(color: onVar),
    em: base.em?.copyWith(color: onVar, fontStyle: FontStyle.italic),
    strong: base.strong?.copyWith(color: onVar, fontWeight: FontWeight.w600),
    a: base.a?.copyWith(color: onVar, decoration: TextDecoration.underline),
    code: base.code?.copyWith(
      color: onVar,
      fontFamily: 'monospace',
      fontSize: 13,
      backgroundColor: Colors.transparent,
    ),
    listIndent: 24,
    listBullet: base.listBullet?.copyWith(color: onVar),
    blockSpacing: 10,
    blockquote: base.blockquote?.copyWith(
      color: onVar.withValues(alpha: 0.92),
      height: 1.45,
    ),
    blockquoteDecoration: BoxDecoration(
      border: Border(
        left: BorderSide(color: onVar.withValues(alpha: 0.45), width: 4),
      ),
    ),
    blockquotePadding: const EdgeInsets.only(left: 12, top: 2, bottom: 2),
    codeblockPadding: const EdgeInsets.all(10),
    codeblockDecoration: BoxDecoration(
      color: Colors.transparent,
      borderRadius: BorderRadius.circular(8),
    ),
    horizontalRuleDecoration: BoxDecoration(
      border: Border(top: BorderSide(color: cs.outlineVariant, width: 1)),
    ),
    tableBorder: TableBorder.all(color: cs.outlineVariant, width: 1),
    tableCellsPadding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
    tableHead: base.tableHead?.copyWith(
      color: onVar,
      fontWeight: FontWeight.w600,
    ),
    tableBody: base.tableBody?.copyWith(color: onVar),
  );
}

BorderRadius _bubbleRadius(bool isUser) {
  const r = 20.0;
  const tail = 8.0;
  return BorderRadius.only(
    topLeft: const Radius.circular(r),
    topRight: const Radius.circular(r),
    bottomLeft: Radius.circular(isUser ? r : tail),
    bottomRight: Radius.circular(isUser ? tail : r),
  );
}

class _AssistantReasoningToggleButton extends StatelessWidget {
  const _AssistantReasoningToggleButton({
    required this.expanded,
    required this.onPressed,
    required this.messageTextColor,
  });

  final bool expanded;
  final VoidCallback onPressed;
  final Color messageTextColor;

  @override
  Widget build(BuildContext context) {
    final tip = expanded ? 'Свернуть размышление' : 'Показать размышление';
    return Semantics(
      excludeSemantics: true,
      label: tip,
      button: true,
      child: IconButton(
        onPressed: onPressed,
        tooltip: tip,
        padding: EdgeInsets.zero,
        visualDensity: VisualDensity.compact,
        constraints: const BoxConstraints(
          minWidth: 36,
          minHeight: 36,
        ),
        icon: Icon(
          expanded ? Icons.expand_less : Icons.expand_more,
          size: 22,
          color: messageTextColor.withValues(alpha: 0.55),
        ),
      ),
    );
  }
}

class _AssistantReasoningExpandedPanel extends StatelessWidget {
  const _AssistantReasoningExpandedPanel({
    super.key,
    required this.theme,
    required this.text,
    required this.messageTextColor,
    required this.padBefore,
  });

  final ThemeData theme;
  final String text;
  final Color messageTextColor;

  final bool padBefore;

  @override
  Widget build(BuildContext context) {
    final cs = theme.colorScheme;
    final bodyStyle = TextStyle(
      fontSize: 13,
      height: 1.45,
      color: messageTextColor.withValues(alpha: 0.78),
      fontFamily: 'monospace',
    );

    return Padding(
      padding: EdgeInsets.only(
        top: padBefore ? 8 : 0,
        bottom: 1,
      ),
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: cs.surfaceContainerHighest.withValues(alpha: 0.42),
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: cs.outlineVariant.withValues(alpha: 0.45),
          ),
        ),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(10, 8, 10, 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  Icon(
                    Icons.psychology_outlined,
                    size: 16,
                    color: messageTextColor.withValues(alpha: 0.82),
                  ),
                  const SizedBox(width: 6),
                  Text(
                    'Размышление',
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: messageTextColor.withValues(alpha: 0.82),
                      height: 1.15,
                      letterSpacing: 0.08,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              SelectableText(
                text,
                style: bodyStyle,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class ChatBubble extends StatefulWidget {
  final Message message;
  final int? sessionId;
  final bool isStreaming;
  final String? streamingStatus;
  final VoidCallback? onRegenerate;
  final Future<void> Function(String newContent)? onEditSubmit;
  final bool showEditNav;
  final int? editsIndex;
  final int? editsTotal;
  final VoidCallback? onPrevEdit;
  final VoidCallback? onNextEdit;
  final bool showContinuePartial;
  final String? streamingReasoning;
  final Map<String, RagDocumentPreview> ragPreviewBySessionFile;

  const ChatBubble({
    super.key,
    required this.message,
    this.sessionId,
    this.isStreaming = false,
    this.streamingStatus,
    this.streamingReasoning,
    this.onRegenerate,
    this.onEditSubmit,
    this.showEditNav = false,
    this.editsIndex,
    this.editsTotal,
    this.onPrevEdit,
    this.onNextEdit,
    this.showContinuePartial = false,
    this.ragPreviewBySessionFile = const {},
  });

  @override
  State<ChatBubble> createState() => _ChatBubbleState();
}

class _ChatBubbleState extends State<ChatBubble> {
  bool _justCopied = false;
  int? _downloadingFileId;
  bool _isEditing = false;
  bool _reasoningExpanded = false;
  Uint8List? _userAttachmentThumbBytes;
  bool _userAttachmentThumbLoading = false;

  @override
  void initState() {
    super.initState();
    _reasoningExpanded = widget.isStreaming;
    unawaited(_maybeLoadUserAttachmentThumb());
  }

  @override
  void didUpdateWidget(ChatBubble oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.message.id != oldWidget.message.id ||
        widget.message.attachmentFileId != oldWidget.message.attachmentFileId ||
        widget.message.attachmentFileName != oldWidget.message.attachmentFileName ||
        widget.message.attachmentMime != oldWidget.message.attachmentMime) {
      _userAttachmentThumbBytes = null;
      unawaited(_maybeLoadUserAttachmentThumb());
    }
    if (widget.message.id != oldWidget.message.id) {
      _reasoningExpanded = widget.isStreaming;
    } else if (!oldWidget.isStreaming && widget.isStreaming) {
      _reasoningExpanded = true;
    } else if (oldWidget.isStreaming && !widget.isStreaming) {
      _reasoningExpanded = false;
    }
  }

  String _reasoningDisplayText(String? tagFromContent) {
    final live = widget.streamingReasoning;
    if (live != null && live.trim().isNotEmpty) {
      return live.trim();
    }
    final stored = widget.message.reasoningContent?.trim() ?? '';
    final tag = tagFromContent?.trim() ?? '';
    return RedactedThinkingSplit.combine(stored, tag).trim();
  }

  Widget _assistantMessageAndReasoningToggle({
    required ThemeData theme,
    required Color messageTextColor,
    required String assistantVisible,
    required bool hasAssistantReasoning,
    required String reasoningDisplay,
    required int messageId,
    required bool enableMarkdownParseGuard,
  }) {
    final hasBody = assistantVisible.trim().isNotEmpty;
    if (!hasAssistantReasoning) {
      return hasBody
          ? _assistantMessageBody(
              theme,
              messageTextColor,
              assistantVisible,
              enableMarkdownParseGuard: enableMarkdownParseGuard,
            )
          : const SizedBox.shrink();
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (_reasoningExpanded)
          _AssistantReasoningExpandedPanel(
            key: ValueKey('assistant-reasoning-$messageId'),
            theme: theme,
            text: reasoningDisplay,
            messageTextColor: messageTextColor,
            padBefore: false,
          ),
        if (_reasoningExpanded) const SizedBox(height: 8),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: hasBody
                  ? _assistantMessageBody(
                      theme,
                      messageTextColor,
                      assistantVisible,
                      enableMarkdownParseGuard: enableMarkdownParseGuard,
                    )
                  : const SizedBox.shrink(),
            ),
            const SizedBox(width: 2),
            _AssistantReasoningToggleButton(
              expanded: _reasoningExpanded,
              messageTextColor: messageTextColor,
              onPressed: () {
                setState(() => _reasoningExpanded = !_reasoningExpanded);
              },
            ),
          ],
        ),
      ],
    );
  }

  Future<void> _maybeLoadUserAttachmentThumb() async {
    final m = widget.message;
    if (!messageEligibleForChatImageThumb(m) ||
        !messageHasImageBytesOrFileRef(m)) {
      if (mounted) {
        setState(() => _userAttachmentThumbLoading = false);
      }
      return;
    }

    final inline = m.attachmentContent;
    if (inline != null && inline.isNotEmpty) {
      if (mounted) {
        setState(() {
          _userAttachmentThumbBytes = inline;
          _userAttachmentThumbLoading = false;
        });
      }
      return;
    }

    final sid = widget.sessionId;
    final fid = m.attachmentFileId;
    if (sid == null || fid == null || fid <= 0) {
      return;
    }

    if (mounted) {
      setState(() => _userAttachmentThumbLoading = true);
    }

    try {
      final dl = await sl<GetSessionFileUseCase>()(
        sessionId: sid,
        fileId: fid,
      );
      if (!mounted) {
        return;
      }
      setState(() {
        _userAttachmentThumbBytes = dl.content;
        _userAttachmentThumbLoading = false;
      });
    } on Object catch (_) {
      if (mounted) {
        setState(() {
          _userAttachmentThumbBytes = null;
          _userAttachmentThumbLoading = false;
        });
      }
    }
  }

  void _openFullScreenImagePreview(BuildContext context, Uint8List bytes) {
    final barrierLabel =
        MaterialLocalizations.of(context).modalBarrierDismissLabel;
    showGeneralDialog<void>(
      context: context,
      barrierDismissible: true,
      barrierLabel: barrierLabel,
      pageBuilder: (ctx, animation, secondaryAnimation) {
        return Scaffold(
          backgroundColor: Colors.black.withValues(alpha: 0.93),
          body: SafeArea(
            child: Stack(
              children: [
                Center(
                  child: Semantics(
                    label:
                        'Вложенное изображение на весь экран, масштаб жестами',
                    child: InteractiveViewer(
                      minScale: 0.4,
                      maxScale: 6,
                      child: ExcludeSemantics(
                        child: Image.memory(
                          bytes,
                          fit: BoxFit.contain,
                          gaplessPlayback: true,
                          errorBuilder: (c, _, stackTrace) => Icon(
                            Icons.broken_image_outlined,
                            color: Colors.white.withValues(alpha: 0.7),
                            size: 64,
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
                Positioned(
                  top: 4,
                  right: 4,
                  child: Semantics(
                    excludeSemantics: true,
                    label: 'Закрыть просмотр изображения',
                    button: true,
                    child: IconButton(
                      tooltip: 'Закрыть',
                      onPressed: () => Navigator.of(ctx).pop(),
                      icon: const Icon(Icons.close, color: Colors.white),
                    ),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget? _userImagePreviewChip(Color messageTextColor) {
    final m = widget.message;
    if (!messageEligibleForChatImageThumb(m) ||
        !messageHasImageBytesOrFileRef(m)) {
      return null;
    }

    if (_userAttachmentThumbLoading && _userAttachmentThumbBytes == null) {
      return Semantics(
        label: 'Загрузка превью вложенного изображения',
        child: SizedBox(
          height: 120,
          width: 200,
          child: Center(
            child: SizedBox(
              width: 28,
              height: 28,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: messageTextColor.withValues(alpha: 0.45),
              ),
            ),
          ),
        ),
      );
    }

    final bytes = _userAttachmentThumbBytes;
    if (bytes == null || bytes.isEmpty) {
      return null;
    }

    return Tooltip(
      message: 'Нажмите, чтобы открыть изображение целиком',
      child: Semantics(
        excludeSemantics: true,
        label: 'Вложенное изображение, открыть целиком',
        button: true,
        child: Material(
          color: Colors.transparent,
          child: InkWell(
            onTap: () => _openFullScreenImagePreview(context, bytes),
            borderRadius: BorderRadius.circular(10),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(10),
              child: Image.memory(
                bytes,
                width: 200,
                height: 120,
                fit: BoxFit.cover,
                gaplessPlayback: true,
                errorBuilder: (ctx, _, stack) => SizedBox(
                  width: 200,
                  height: 48,
                  child: Icon(
                    Icons.broken_image_outlined,
                    color: messageTextColor.withValues(alpha: 0.45),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  Future<void> _downloadSessionFile(int fileId) async {
    final sessionId = widget.sessionId;
    if (sessionId == null || fileId <= 0) {
      return;
    }
    setState(() => _downloadingFileId = fileId);
    try {
      final dl = await sl<GetSessionFileUseCase>()(
        sessionId: sessionId,
        fileId: fileId,
      );
      final name = dl.filename;
      final lower = name.toLowerCase();
      var ok = false;
      if (lower.endsWith('.xlsx')) {
        ok = await saveSpreadsheetToFile(dl.content, name);
      } else if (lower.endsWith('.csv') ||
          lower.endsWith('.md') ||
          lower.endsWith('.txt')) {
        ok = await saveCsvToFile(
          utf8.decode(dl.content, allowMalformed: true),
          name,
        );
      } else if (lower.endsWith('.docx')) {
        ok = await saveDocxToFile(dl.content, name);
      } else {
        ok = await saveUserPickedFile(dl.content, name);
      }
      if (!mounted) {
        return;
      }
      if (ok) {
        showAppTopNotice('Сохранено: $name');
      }
    } on Object catch (e) {
      if (!mounted) {
        return;
      }
      Logs().e('ChatBubble: скачивание файла', exception: e);
      showAppTopNotice(
        'Не удалось скачать файл (${userSafeErrorMessage(e, fallback: 'ошибка')})',
        error: true,
      );
    } finally {
      if (mounted) {
        setState(() => _downloadingFileId = null);
      }
    }
  }

  void _showRagPreviewForAttachment(BuildContext context) {
    final sid = widget.sessionId;
    final fid = widget.message.attachmentFileId;
    if (sid == null || fid == null || fid <= 0) {
      return;
    }
    final key = '${sid}_$fid';
    final stored = widget.ragPreviewBySessionFile[key];
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (ctx) {
        final theme = Theme.of(ctx);
        return SafeArea(
          child: stored != null
              ? SingleChildScrollView(
                  child: RagContextPreviewBanner(
                    preview: stored,
                    onDismiss: () => Navigator.of(ctx).pop(),
                  ),
                )
              : Padding(
                  padding: const EdgeInsets.fromLTRB(20, 12, 20, 28),
                  child: Text(
                    'Сохранённое превью будет после ответа с поиском по документу (RAG). '
                    'Отправьте сообщение с тем же файлом и включённым поиском по вложению.',
                    style: theme.textTheme.bodyMedium,
                  ),
                ),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final message = widget.message;
    final isStreaming = widget.isStreaming;
    final isUser = message.role == MessageRole.user;
    final isAssistant = message.role == MessageRole.assistant;
    final isTool = message.role == MessageRole.tool;
    final theme = Theme.of(context);
    final width = Breakpoints.width(context);
    const minBubbleWidth = 64.0;
    final maxBubbleWidth = Breakpoints.isMobile(context)
        ? width * 0.88
        : (Breakpoints.isTablet(context) ? 420.0 : 560.0);
    final semanticsRole = switch (message.role) {
      MessageRole.user => 'Ваше сообщение',
      MessageRole.assistant => 'Ответ ассистента',
      MessageRole.tool => 'Результат инструмента',
    };
    final peeledAssistant = isAssistant
        ? RedactedThinkingSplit.peel(message.content)
        : null;
    final assistantVisible = isAssistant
        ? peeledAssistant!.$1
        : message.content;
    final tagReasoningFromBody = isAssistant ? peeledAssistant!.$2 : null;
    final reasoningDisplay = isAssistant
        ? _reasoningDisplayText(tagReasoningFromBody)
        : '';
    final hasCopyableText = isUser
        ? message.content.trim().isNotEmpty
        : assistantVisible.trim().isNotEmpty;
    final editsTotal = widget.editsTotal;
    final editsIndex = widget.editsIndex;
    final showEditNav = widget.showEditNav;
    final sessionFileIds = isAssistant && !isStreaming && widget.sessionId != null
        ? extractSessionFileIdsFromText(assistantVisible)
        : const <int>[];
    final attachmentLabel =
        message.attachmentFileName ??
        (message.attachmentFileId != null
            ? 'Файл #${message.attachmentFileId}'
            : null);
    final attachmentFileId = message.attachmentFileId;
    final ragPreviewKey =
        widget.sessionId != null &&
            attachmentFileId != null &&
            attachmentFileId > 0
        ? '${widget.sessionId}_$attachmentFileId'
        : null;
    final hasRagPreviewStored =
        ragPreviewKey != null &&
        widget.ragPreviewBySessionFile.containsKey(ragPreviewKey);
    final canOpenRagPreview = ragPreviewKey != null && !isStreaming;
    final messageTextColor = _messageBodyTextColor(theme.colorScheme);
    final hasAssistantReasoning =
        isAssistant && reasoningDisplay.isNotEmpty;
    final userImagePreview =
        isUser ? _userImagePreviewChip(messageTextColor) : null;

    return Semantics(
      container: true,
      label: semanticsRole,
      child: Align(
        alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
        child: Column(
          crossAxisAlignment: isUser
              ? CrossAxisAlignment.end
              : CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            ClipRRect(
              borderRadius: _bubbleRadius(isUser),
              child: Container(
                margin: const EdgeInsets.symmetric(vertical: 2),
                padding: EdgeInsets.symmetric(
                  horizontal: Breakpoints.isMobile(context) ? 12 : 16,
                  vertical: Breakpoints.isMobile(context) ? 10 : 12,
                ),
                constraints: BoxConstraints(
                  minWidth: minBubbleWidth,
                  maxWidth: maxBubbleWidth,
                ),
                decoration: BoxDecoration(
                  color: theme.colorScheme.surfaceContainerHigh,
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (userImagePreview != null) ...[
                      userImagePreview,
                      const SizedBox(height: 8),
                    ],
                    if (attachmentLabel != null)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Semantics(
                          excludeSemantics: true,
                          button: canOpenRagPreview,
                          enabled: canOpenRagPreview,
                          label: canOpenRagPreview
                              ? 'Вложение $attachmentLabel, открыть превью для поиска по документу'
                              : (messageEligibleForChatImageThumb(message)
                                    ? 'Вложение изображение: $attachmentLabel'
                                    : 'Вложение файл: $attachmentLabel'),
                          child: Tooltip(
                            message: canOpenRagPreview
                                ? 'Открыть, как модель видит документ (RAG)'
                                : (messageEligibleForChatImageThumb(message)
                                      ? 'Вложение: изображение'
                                      : 'Вложение: файл'),
                            child: Material(
                              color: Colors.transparent,
                              child: InkWell(
                                onTap: canOpenRagPreview
                                    ? () => _showRagPreviewForAttachment(context)
                                    : null,
                                borderRadius: BorderRadius.circular(8),
                                child: Padding(
                                  padding: const EdgeInsets.symmetric(
                                    vertical: 4,
                                    horizontal: 2,
                                  ),
                                  child: Row(
                                    mainAxisSize: MainAxisSize.min,
                                    children: [
                                      Icon(
                                        messageEligibleForChatImageThumb(
                                          message,
                                        )
                                            ? Icons.image_outlined
                                            : Icons.insert_drive_file_rounded,
                                        size: 18,
                                        color: messageTextColor,
                                      ),
                                      const SizedBox(width: 6),
                                      Flexible(
                                        child: Text(
                                          attachmentLabel,
                                          style: TextStyle(
                                            fontSize: 13,
                                            color: messageTextColor,
                                            decoration: canOpenRagPreview
                                                ? TextDecoration.underline
                                                : TextDecoration.none,
                                            decorationStyle:
                                                TextDecorationStyle.dotted,
                                          ),
                                          overflow: TextOverflow.ellipsis,
                                        ),
                                      ),
                                      if (canOpenRagPreview) ...[
                                        const SizedBox(width: 4),
                                        Tooltip(
                                          message: hasRagPreviewStored
                                              ? 'Как модель видит документ'
                                              : 'Превью после ответа с RAG',
                                          child: Icon(
                                            hasRagPreviewStored
                                                ? Icons.visibility_outlined
                                                : Icons.visibility_off_outlined,
                                            size: 18,
                                            color: messageTextColor.withValues(
                                              alpha: 0.75,
                                            ),
                                          ),
                                        ),
                                      ],
                                    ],
                                  ),
                                ),
                              ),
                            ),
                          ),
                        ),
                      ),
                    if (_isEditing && isUser && !isStreaming)
                      ChatInputBar(
                        key: ValueKey('edit-${message.id}-${message.content}'),
                        isEnabled: widget.onEditSubmit != null,
                        initialText: message.content,
                        allowAttachments: false,
                        showRetry: false,
                        showStop: false,
                        clearOnSubmit: false,
                        roundedCard: true,
                        onCancel: () {
                          setState(() => _isEditing = false);
                        },
                        onSubmitText: widget.onEditSubmit == null
                            ? null
                            : (text) async {
                                final raw = text;
                                final trimmed = raw.trim();

                                if (trimmed.isEmpty) {
                                  if (!mounted) {
                                    return;
                                  }

                                  showAppTopNotice(
                                    'Текст не может быть пустым',
                                    error: true,
                                  );
                                  return;
                                }

                                await widget.onEditSubmit!(trimmed);
                                if (!mounted) {
                                  return;
                                }
                                setState(() => _isEditing = false);
                              },
                      )
                    else if (isUser
                        ? message.content.isNotEmpty
                        : (assistantVisible.trim().isNotEmpty ||
                            reasoningDisplay.isNotEmpty))
                      isUser
                          ? SelectableText(
                              message.content,
                              style: TextStyle(
                                color: messageTextColor,
                                fontSize: 15,
                                height: 1.5,
                              ),
                            )
                          : (isAssistant
                                ? _assistantMessageAndReasoningToggle(
                              theme: theme,
                              messageTextColor: messageTextColor,
                              assistantVisible: assistantVisible,
                              hasAssistantReasoning: hasAssistantReasoning,
                              reasoningDisplay: reasoningDisplay,
                              messageId: message.id,
                              enableMarkdownParseGuard: true,
                            )
                                : SelectableText(
                                    assistantVisible,
                                    style: TextStyle(
                                      color: messageTextColor,
                                      fontSize: 14,
                                      height: 1.45,
                                      fontFamily: 'monospace',
                                    ),
                                  )),
                    if (isStreaming &&
                        (widget.streamingStatus?.trim().isNotEmpty ?? false))
                      Padding(
                        padding: EdgeInsets.only(
                          bottom: 8,
                          top: (isUser
                                  ? message.content.trim().isNotEmpty
                                  : (assistantVisible.trim().isNotEmpty ||
                                      reasoningDisplay.isNotEmpty))
                              ? 8
                              : 0,
                        ),
                        child: _ToolProgressLine(
                          text: widget.streamingStatus!.trim(),
                          messageTextColor: messageTextColor,
                        ),
                      ),
                    if (isStreaming)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Semantics(
                          excludeSemantics: true,
                          label: 'Обрабатываю ответ',
                          child: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              SizedBox(
                                width: 12,
                                height: 12,
                                child: CircularProgressIndicator(
                                  strokeWidth: 1.5,
                                  color: messageTextColor.withValues(
                                    alpha: 0.75,
                                  ),
                                ),
                              ),
                              const SizedBox(width: 8),
                              Text(
                                'Обрабатываю...',
                                style: TextStyle(
                                  fontSize: 12,
                                  height: 1.2,
                                  color: messageTextColor.withValues(
                                    alpha: 0.75,
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    if (sessionFileIds.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(top: 10),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              'Файл можно скачать и открыть во внешней программе.',
                              style: TextStyle(
                                fontSize: 12,
                                height: 1.3,
                                color: messageTextColor.withValues(alpha: 0.72),
                              ),
                            ),
                            const SizedBox(height: 8),
                            Wrap(
                              spacing: 8,
                              runSpacing: 6,
                              children: [
                                for (final fid in sessionFileIds)
                                  Tooltip(
                                    message:
                                        'Скачать артефакт с сервера (в приложении превью нет)',
                                    child: Semantics(
                                      excludeSemantics: true,
                                      label:
                                          'Скачать файл #$fid с сервера, открыть во внешней программе',
                                      button: true,
                                      enabled: _downloadingFileId == null,
                                      child: TextButton.icon(
                                        onPressed: _downloadingFileId != null
                                            ? null
                                            : () => _downloadSessionFile(fid),
                                        icon: _downloadingFileId == fid
                                            ? SizedBox(
                                                width: 16,
                                                height: 16,
                                                child: CircularProgressIndicator(
                                                  strokeWidth: 2,
                                                  color: messageTextColor
                                                      .withValues(alpha: 0.85),
                                                ),
                                              )
                                            : Icon(
                                                Icons.download_rounded,
                                                size: 18,
                                                color:
                                                    theme.colorScheme.primary,
                                              ),
                                        label: Text(
                                          'Файл #$fid',
                                          style: TextStyle(
                                            fontSize: 13,
                                            color: theme.colorScheme.primary,
                                          ),
                                        ),
                                        style: TextButton.styleFrom(
                                          padding: const EdgeInsets.symmetric(
                                            horizontal: 10,
                                            vertical: 6,
                                          ),
                                          visualDensity: VisualDensity.compact,
                                        ),
                                      ),
                                    ),
                                  ),
                              ],
                            ),
                          ],
                        ),
                      ),
                  ],
                ),
              ),
            ),
            if (hasCopyableText ||
                isStreaming ||
                widget.onRegenerate != null ||
                widget.onEditSubmit != null ||
                showEditNav ||
                widget.showContinuePartial)
              Padding(
                padding: const EdgeInsets.only(
                  left: 4,
                  right: 4,
                  top: 2,
                  bottom: 4,
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (showEditNav) ...[
                      Semantics(
                        excludeSemantics: true,
                        label: 'Предыдущая версия сообщения',
                        button: true,
                        enabled: widget.onPrevEdit != null,
                        child: IconButton(
                          onPressed: widget.onPrevEdit,
                          icon: const Icon(Icons.chevron_left_rounded, size: 20),
                          tooltip: 'Предыдущая версия',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                        ),
                      ),
                      Semantics(
                        label:
                            'Версия сообщения ${(editsIndex ?? 0) + 1} из ${editsTotal ?? 1}',
                        child: Text(
                          '${(editsIndex ?? 0) + 1}/${editsTotal ?? 1}',
                          style: TextStyle(
                            fontSize: 12,
                            color: theme.colorScheme.onSurfaceVariant.withValues(
                              alpha: 0.9,
                            ),
                          ),
                        ),
                      ),
                      Semantics(
                        excludeSemantics: true,
                        label: 'Следующая версия сообщения',
                        button: true,
                        enabled: widget.onNextEdit != null,
                        child: IconButton(
                          onPressed: widget.onNextEdit,
                          icon: const Icon(Icons.chevron_right_rounded, size: 20),
                          tooltip: 'Следующая версия',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                    ],
                    if (hasCopyableText || isStreaming)
                      Semantics(
                        excludeSemantics: true,
                        label: _justCopied
                            ? 'Текст скопирован в буфер обмена'
                            : (hasCopyableText
                                  ? 'Копировать текст сообщения в буфер обмена'
                                  : 'Копирование недоступно, ответ ещё формируется'),
                        button: true,
                        enabled: hasCopyableText,
                        child: IconButton(
                          onPressed: hasCopyableText
                              ? () async {
                                  await Clipboard.setData(
                                    ClipboardData(
                                      text: isUser
                                          ? message.content
                                          : assistantVisible.trim(),
                                    ),
                                  );

                                  if (!mounted) {
                                    return;
                                  }

                                  setState(() => _justCopied = true);

                                  Future.delayed(const Duration(seconds: 2), () {
                                    if (mounted) {
                                      setState(() => _justCopied = false);
                                    }
                                  });
                                }
                              : null,
                          icon: Icon(
                            _justCopied
                                ? Icons.check_rounded
                                : Icons.copy_rounded,
                            size: 18,
                            color: theme.colorScheme.onSurfaceVariant.withValues(
                              alpha: hasCopyableText ? 1 : 0.4,
                            ),
                          ),
                          tooltip: _justCopied ? 'Скопировано' : 'Копировать',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                          style: IconButton.styleFrom(
                            foregroundColor: theme.colorScheme.onSurfaceVariant
                                .withValues(alpha: hasCopyableText ? 1 : 0.4),
                          ),
                        ),
                      ),
                    if (widget.showContinuePartial) ...[
                      const SizedBox(width: 4),
                      Semantics(
                        excludeSemantics: true,
                        label: 'Продолжить ответ ассистента',
                        button: true,
                        enabled: message.id > 0,
                        child: IconButton(
                          onPressed: message.id > 0
                              ? () => context.read<ChatBloc>().add(
                                  ChatContinueAssistant(message.id),
                                )
                              : null,
                          icon: const Icon(Icons.play_arrow_rounded, size: 18),
                          tooltip: 'Продолжить',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                        ),
                      ),
                    ],
                    if (widget.onRegenerate != null)
                      Semantics(
                        excludeSemantics: true,
                        label: 'Перегенерировать ответ ассистента',
                        button: true,
                        child: IconButton(
                          onPressed: widget.onRegenerate,
                          icon: const Icon(Icons.refresh_rounded, size: 18),
                          tooltip: 'Перегенерировать ответ',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                        ),
                      ),
                    if (widget.onEditSubmit != null && isUser && !isStreaming)
                      Semantics(
                        excludeSemantics: true,
                        label: _isEditing
                            ? 'Закончить редактирование сообщения'
                            : 'Редактировать сообщение и продолжить диалог',
                        button: true,
                        child: IconButton(
                          onPressed: () {
                            setState(() => _isEditing = !_isEditing);
                          },
                          icon: const Icon(Icons.edit_rounded, size: 18),
                          tooltip: 'Редактировать и продолжить',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                        ),
                      ),
                    if (isTool && message.toolName != null && message.toolName!.trim().isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(left: 6),
                        child: Text(
                          message.toolName!.trim(),
                          style: TextStyle(
                            fontSize: 11,
                            color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.75),
                            fontStyle: FontStyle.italic,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}

final _mcpToolImageBlock = RegExp(
  r'\[Изображение mime="([^"]+)" base64\]\s*\n([A-Za-z0-9+/=]+)',
  multiLine: true,
);

final _markdownDataImage = RegExp(
  r'!\[([^\]]*)\]\(data:image/([\w.+-]+);base64,([A-Za-z0-9+/=]+)\)',
);

abstract class _AssistantBodySeg {}

class _AssistantTextSeg implements _AssistantBodySeg {
  _AssistantTextSeg(this.text);
  final String text;
}

class _AssistantImageSeg implements _AssistantBodySeg {
  _AssistantImageSeg(this.bytes);
  final Uint8List bytes;
}

Uint8List? _decodeLooseBase64(String raw) {
  try {
    final clean = raw.replaceAll(RegExp(r'\s+'), '');
    if (clean.isEmpty) {
      return null;
    }
    return Uint8List.fromList(base64Decode(clean));
  } catch (_) {
    return null;
  }
}

List<_AssistantBodySeg> _parseAssistantBodySegments(String input) {
  final out = <_AssistantBodySeg>[];
  var rest = input;
  while (rest.isNotEmpty) {
    final mTool = _mcpToolImageBlock.firstMatch(rest);
    final mMd = _markdownDataImage.firstMatch(rest);
    final iTool = mTool?.start ?? rest.length + 1;
    final iMd = mMd?.start ?? rest.length + 1;
    if (mTool == null && mMd == null) {
      out.add(_AssistantTextSeg(rest));
      break;
    }

    final useTool = mTool != null && iTool <= iMd;
    final m = useTool ? mTool : mMd!;
    if (m.start > 0) {
      out.add(_AssistantTextSeg(rest.substring(0, m.start)));
    }

    if (useTool) {
      final b64 = m.group(2)!;
      final bytes = _decodeLooseBase64(b64);
      if (bytes != null && bytes.isNotEmpty) {
        out.add(_AssistantImageSeg(bytes));
      } else {
        out.add(_AssistantTextSeg(m.group(0)!));
      }
    } else {
      final b64 = m.group(3)!;
      final bytes = _decodeLooseBase64(b64);
      if (bytes != null && bytes.isNotEmpty) {
        out.add(_AssistantImageSeg(bytes));
      } else {
        out.add(_AssistantTextSeg(m.group(0)!));
      }
    }
    rest = rest.substring(m.end);
  }

  return out;
}

Widget _assistantMessageBody(
  ThemeData theme,
  Color messageTextColor,
  String content, {
  required bool enableMarkdownParseGuard,
}) {
  final sheet = assistantBubbleMarkdownSheet(theme);
  final preBuilder = CodeBlockBuilder(
    textStyle: TextStyle(
      fontSize: 13,
      fontFamily: 'monospace',
      color: messageTextColor,
    ),
  );
  final builders = <String, MarkdownElementBuilder>{'pre': preBuilder};
  final fallbackStyle =
      sheet.p?.copyWith(color: messageTextColor) ??
      TextStyle(
        fontSize: 15,
        height: 1.5,
        color: messageTextColor,
      );

  Widget mdBody(String data) {
    if (enableMarkdownParseGuard) {
      return SafeMarkdownBody(
        data: data,
        selectable: true,
        styleSheet: sheet,
        builders: builders,
        fallbackStyle: fallbackStyle,
      );
    }
    return MarkdownBody(
      data: data,
      selectable: true,
      styleSheet: sheet,
      builders: builders,
    );
  }

  final segs = _parseAssistantBodySegments(content);
  if (segs.length == 1 && segs.first is _AssistantTextSeg) {
    return mdBody((segs.first as _AssistantTextSeg).text);
  }

  return Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      for (final s in segs)
        if (s is _AssistantTextSeg && s.text.trim().isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: mdBody(s.text),
          )
        else if (s is _AssistantImageSeg)
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxHeight: 360),
                child: Image.memory(
                  s.bytes,
                  fit: BoxFit.contain,
                  semanticLabel: 'Изображение в ответе ассистента',
                  errorBuilder: (context, error, stackTrace) => Semantics(
                    excludeSemantics: true,
                    label: 'Не удалось показать изображение',
                    child: Text(
                      'Не удалось показать изображение',
                      style: TextStyle(
                        fontSize: 13,
                        color: messageTextColor.withValues(alpha: 0.8),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
    ],
  );
}

class _ToolProgressLine extends StatelessWidget {
  const _ToolProgressLine({required this.text, required this.messageTextColor});

  final String text;
  final Color messageTextColor;

  @override
  Widget build(BuildContext context) {
    final lower = text.toLowerCase();
    final isMcp = text.contains('MCP') || text.contains('MCP #');
    final isWebSearch = lower.contains('web_search');

    return Semantics(
      container: true,
      label: text,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (isMcp)
            ExcludeSemantics(
              child: Padding(
                padding: const EdgeInsets.only(top: 1),
                child: Icon(
                  Icons.extension_outlined,
                  size: 16,
                  color: messageTextColor.withValues(alpha: 0.8),
                ),
              ),
            )
          else if (isWebSearch)
            ExcludeSemantics(
              child: Padding(
                padding: const EdgeInsets.only(top: 1),
                child: Icon(
                  Icons.travel_explore,
                  size: 16,
                  color: messageTextColor.withValues(alpha: 0.8),
                ),
              ),
            ),
          if (isMcp || isWebSearch) const SizedBox(width: 8),
          Expanded(
            child: ExcludeSemantics(
              child: Text(
                text,
                style: TextStyle(
                  fontSize: 13,
                  height: 1.35,
                  color: messageTextColor.withValues(alpha: 0.85),
                  fontStyle: FontStyle.italic,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
