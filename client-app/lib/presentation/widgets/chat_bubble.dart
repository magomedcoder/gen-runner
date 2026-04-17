import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:gen/core/redacted_thinking_split.dart';
import 'package:gen/core/docx_file_export.dart';
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

class _AssistantReasoningCard extends StatelessWidget {
  const _AssistantReasoningCard({
    super.key,
    required this.theme,
    required this.text,
    required this.messageTextColor,
    required this.maxWidth,
    this.embedded = false,
    this.initiallyExpanded = false,
  });

  final ThemeData theme;
  final String text;
  final Color messageTextColor;
  final double maxWidth;
  final bool embedded;
  final bool initiallyExpanded;

  @override
  Widget build(BuildContext context) {
    final cs = theme.colorScheme;
    return Container(
      margin: embedded
          ? EdgeInsets.zero
          : const EdgeInsets.only(bottom: 8, top: 2),
      constraints: BoxConstraints(maxWidth: maxWidth),
      decoration: BoxDecoration(
        color: embedded
            ? cs.surfaceContainerLow.withValues(alpha: 0.65)
            : cs.surfaceContainerLow,
        borderRadius: embedded ? BorderRadius.zero : BorderRadius.circular(14),
        border: embedded
            ? null
            : Border.all(color: cs.outlineVariant.withValues(alpha: 0.55)),
      ),
      child: ClipRRect(
        borderRadius: embedded ? BorderRadius.zero : BorderRadius.circular(13),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ColoredBox(
              color: cs.tertiary.withValues(alpha: 0.9),
              child: const SizedBox(width: 4),
            ),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(10, 8, 8, 8),
                child: Theme(
                  data: theme.copyWith(dividerColor: Colors.transparent),
                  child: ExpansionTile(
                    tilePadding: EdgeInsets.zero,
                    expandedAlignment: Alignment.centerLeft,
                    childrenPadding: const EdgeInsets.only(top: 6),
                    initiallyExpanded: initiallyExpanded,
                    collapsedShape: const RoundedRectangleBorder(),
                    shape: const RoundedRectangleBorder(),
                    title: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Icon(
                          Icons.psychology_outlined,
                          size: 20,
                          color: cs.tertiary,
                        ),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(
                            'Размышление',
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w600,
                              color: messageTextColor.withValues(alpha: 0.88),
                              height: 1.25,
                            ),
                          ),
                        ),
                      ],
                    ),
                    children: [
                      SelectableText(
                        text,
                        style: TextStyle(
                          fontSize: 13,
                          height: 1.45,
                          color: messageTextColor.withValues(alpha: 0.76),
                          fontFamily: 'monospace',
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ],
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

  String _reasoningDisplayText(String? tagFromContent) {
    final live = widget.streamingReasoning;
    if (live != null && live.trim().isNotEmpty) {
      return live.trim();
    }
    final stored = widget.message.reasoningContent?.trim() ?? '';
    final tag = tagFromContent?.trim() ?? '';
    return RedactedThinkingSplit.combine(stored, tag).trim();
  }

  static String _copyAssistantClipboard(String visible, String reasoning) {
    final v = visible.trim();
    final r = reasoning.trim();
    if (r.isEmpty) {
      return v;
    }
    if (v.isEmpty) {
      return r;
    }
    return '$v\n\n---\nРазмышление:\n$r';
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
    final theme = Theme.of(context);
    final width = Breakpoints.width(context);
    const minBubbleWidth = 64.0;
    final maxBubbleWidth = Breakpoints.isMobile(context)
        ? width * 0.88
        : (Breakpoints.isTablet(context) ? 420.0 : 560.0);
    final semanticsRole = isUser ? 'Ваше сообщение' : 'Ответ ассистента';
    final peeledAssistant = !isUser
        ? RedactedThinkingSplit.peel(message.content)
        : null;
    final assistantVisible =
        isUser ? message.content : peeledAssistant!.$1;
    final tagReasoningFromBody = !isUser ? peeledAssistant!.$2 : null;
    final reasoningDisplay = !isUser
        ? _reasoningDisplayText(tagReasoningFromBody)
        : '';
    final hasCopyableText = isUser
        ? message.content.trim().isNotEmpty
        : (assistantVisible.trim().isNotEmpty || reasoningDisplay.isNotEmpty);
    final editsTotal = widget.editsTotal;
    final editsIndex = widget.editsIndex;
    final showEditNav = widget.showEditNav;
    final sessionFileIds = !isUser && !isStreaming && widget.sessionId != null
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
        !isUser && reasoningDisplay.isNotEmpty;

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
                    if (attachmentLabel != null)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
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
                                    Icons.insert_drive_file_rounded,
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
                          : (assistantVisible.trim().isNotEmpty
                              ? _assistantMessageBody(
                                  theme,
                                  messageTextColor,
                                  assistantVisible,
                                )
                              : const SizedBox.shrink()),
                    if (hasAssistantReasoning) ...[
                      if (assistantVisible.trim().isNotEmpty)
                        Divider(
                          height: 20,
                          thickness: 1,
                          color: theme.colorScheme.outlineVariant.withValues(
                            alpha: 0.45,
                          ),
                        ),
                      _AssistantReasoningCard(
                        key: ValueKey(
                          'assistant-reasoning-${message.id}-$isStreaming',
                        ),
                        theme: theme,
                        text: reasoningDisplay,
                        messageTextColor: messageTextColor,
                        maxWidth: maxBubbleWidth,
                        embedded: true,
                        initiallyExpanded: isStreaming,
                      ),
                    ],
                    if (isStreaming &&
                        (widget.streamingStatus?.trim().isNotEmpty ?? false))
                      Padding(
                        padding: EdgeInsets.only(
                          bottom: 8,
                          top: (isUser
                                  ? message.content
                                  : assistantVisible)
                              .trim()
                              .isNotEmpty
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
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            SizedBox(
                              width: 12,
                              height: 12,
                              child: CircularProgressIndicator(
                                strokeWidth: 1.5,
                                color: messageTextColor.withValues(alpha: 0.75),
                              ),
                            ),
                            const SizedBox(width: 8),
                            Text(
                              'Обрабатываю...',
                              style: TextStyle(
                                fontSize: 12,
                                height: 1.2,
                                color: messageTextColor.withValues(alpha: 0.75),
                              ),
                            ),
                          ],
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
                                              color: theme.colorScheme.primary,
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
                      IconButton(
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
                      Text(
                        '${(editsIndex ?? 0) + 1}/${editsTotal ?? 1}',
                        style: TextStyle(
                          fontSize: 12,
                          color: theme.colorScheme.onSurfaceVariant.withValues(
                            alpha: 0.9,
                          ),
                        ),
                      ),
                      IconButton(
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
                      const SizedBox(width: 8),
                    ],
                    if (hasCopyableText || isStreaming)
                      IconButton(
                        onPressed: hasCopyableText
                            ? () async {
                                await Clipboard.setData(
                                  ClipboardData(
                                    text: isUser
                                        ? message.content
                                        : _copyAssistantClipboard(
                                            assistantVisible,
                                            reasoningDisplay,
                                          ),
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
                    if (widget.showContinuePartial) ...[
                      const SizedBox(width: 4),
                      IconButton(
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
                    ],
                    if (widget.onRegenerate != null)
                      IconButton(
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
                    if (widget.onEditSubmit != null && isUser && !isStreaming)
                      IconButton(
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
  String content,
) {
  final sheet = assistantBubbleMarkdownSheet(theme);
  final preBuilder = CodeBlockBuilder(
    textStyle: TextStyle(
      fontSize: 13,
      fontFamily: 'monospace',
      color: messageTextColor,
    ),
  );
  final builders = <String, MarkdownElementBuilder>{'pre': preBuilder};

  final segs = _parseAssistantBodySegments(content);
  if (segs.length == 1 && segs.first is _AssistantTextSeg) {
    return MarkdownBody(
      data: (segs.first as _AssistantTextSeg).text,
      selectable: true,
      styleSheet: sheet,
      builders: builders,
    );
  }

  return Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      for (final s in segs)
        if (s is _AssistantTextSeg && s.text.trim().isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: MarkdownBody(
              data: s.text,
              selectable: true,
              styleSheet: sheet,
              builders: builders,
            ),
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
                  errorBuilder: (context, error, stackTrace) => Text(
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

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (isMcp)
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: Icon(
              Icons.extension_outlined,
              size: 16,
              color: messageTextColor.withValues(alpha: 0.8),
            ),
          )
        else if (isWebSearch)
          Padding(
            padding: const EdgeInsets.only(top: 1),
            child: Icon(
              Icons.travel_explore,
              size: 16,
              color: messageTextColor.withValues(alpha: 0.8),
            ),
          ),
        if (isMcp || isWebSearch) const SizedBox(width: 8),
        Expanded(
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
      ],
    );
  }
}
