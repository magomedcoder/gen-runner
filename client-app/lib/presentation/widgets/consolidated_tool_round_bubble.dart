import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/core/redacted_thinking_split.dart';
import 'package:gen/core/tool_display_format.dart';
import 'package:gen/domain/entities/message.dart';
import 'package:gen/presentation/widgets/chat_bubble.dart';

class ToolChainSegmentView {
  const ToolChainSegmentView({
    required this.leadAssistant,
    required this.toolMessages,
  });

  final Message leadAssistant;
  final List<Message> toolMessages;
}

class ConsolidatedToolRoundBubble extends StatefulWidget {
  const ConsolidatedToolRoundBubble({
    super.key,
    required this.segments,
    this.finalAssistant,
    this.onRegenerate,
    this.onContinueAssistant,
    this.showContinuePartial = false,
    this.showEditNav = false,
    this.editsIndex,
    this.editsTotal,
    this.onPrevEdit,
    this.onNextEdit,
  });

  final List<ToolChainSegmentView> segments;
  final Message? finalAssistant;

  final VoidCallback? onRegenerate;
  final VoidCallback? onContinueAssistant;
  final bool showContinuePartial;
  final bool showEditNav;
  final int? editsIndex;
  final int? editsTotal;
  final VoidCallback? onPrevEdit;
  final VoidCallback? onNextEdit;

  @override
  State<ConsolidatedToolRoundBubble> createState() => _ConsolidatedToolRoundBubbleState();
}

class _ConsolidatedToolRoundBubbleState extends State<ConsolidatedToolRoundBubble> {
  bool _justCopied = false;

  int get _totalTools => widget.segments.fold<int>(0, (a, s) => a + s.toolMessages.length);

  String _copyablePlainText() {
    final b = StringBuffer();
    final tail = widget.finalAssistant;
    (String, String?)? peeledTail;
    if (tail != null) {
      final p = RedactedThinkingSplit.peel(tail.content);
      peeledTail = p;
      final s1 = tail.reasoningContent?.trim() ?? '';
      final s2 = (p.$2 ?? '').trim();
      if (s1.isNotEmpty) {
        b.writeln('Размышление');
        b.writeln(s1);
        b.writeln();
      }

      if (s2.isNotEmpty) {
        b.writeln('Размышление модели');
        b.writeln(s2);
        b.writeln();
      }
    }

    b.writeln('Инструменты');
    var round = 0;
    for (final seg in widget.segments) {
      round++;
      if (widget.segments.length > 1) {
        b.writeln('Шаг $round');
      }

      final names = parseToolCallNamesFromToolCallsJson(
        seg.leadAssistant.toolCallsJson,
      );

      for (var i = 0; i < seg.toolMessages.length; i++) {
        final tm = seg.toolMessages[i];
        final label = (tm.toolName?.trim().isNotEmpty ?? false)
            ? tm.toolName!.trim()
            : (i < names.length ? names[i] : 'Вызов ${i + 1}');
        b.writeln('- $label');
        b.writeln(formatToolResultForUser(tm.content));
        b.writeln();
      }
    }

    b.writeln('Размещение');
    b.writeln(_placementBody());
    if (tail != null && peeledTail != null) {
      b.writeln();
      b.writeln('Ответ');
      b.write(peeledTail.$1.trim());
    }

    return b.toString().trim();
  }

  String _placementBody() {
    final b = StringBuffer();
    var r = 0;
    for (final seg in widget.segments) {
      r++;
      if (widget.segments.length > 1) {
        b.writeln('=== Шаг $r ===');
        b.writeln();
      }

      final tc = seg.leadAssistant.toolCallsJson?.trim();
      if (tc != null && tc.isNotEmpty) {
        b.writeln('tool_calls_json');
        b.writeln(_prettyOrRaw(tc));
        b.writeln();
      }

      final lead = seg.leadAssistant.content.trim();
      if (lead.isNotEmpty) {
        b.writeln('Черновик модели (сырой текст)');
        b.writeln(lead);
        b.writeln();
      }

      for (var i = 0; i < seg.toolMessages.length; i++) {
        final tm = seg.toolMessages[i];
        b.writeln('Результат инструмента $r.${i + 1} (${tm.toolName ?? tm.toolCallId ?? ''})');
        b.writeln(tm.content.trim());
        b.writeln();
      }
    }

    return b.toString().trim();
  }

  static String _prettyOrRaw(String raw) {
    try {
      final o = json.decode(raw);
      return const JsonEncoder.withIndent('  ').convert(o);
    } catch (_) {
      return raw;
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final width = Breakpoints.width(context);
    const minBubbleWidth = 64.0;
    final maxBubbleWidth = Breakpoints.isMobile(context)
        ? width * 0.88
        : (Breakpoints.isTablet(context) ? 420.0 : 560.0);
    final messageTextColor = theme.colorScheme.onSurface.withValues(alpha: 0.94);
    final tileTitleStyle = TextStyle(
      fontSize: 12,
      color: theme.colorScheme.onSurfaceVariant,
    );

    final monoReason = TextStyle(
      fontSize: 12,
      height: 1.4,
      color: messageTextColor.withValues(alpha: 0.85),
      fontFamily: 'monospace',
    );

    final tail = widget.finalAssistant;
    final peeledTail = tail != null ? RedactedThinkingSplit.peel(tail.content) : null;
    final tailBody = peeledTail?.$1 ?? '';
    final storedReasoning = tail?.reasoningContent?.trim() ?? '';
    final tagThinkingTrim = (peeledTail?.$2 ?? '').trim();

    final hasMainText = tailBody.trim().isNotEmpty;
    final hasCopyable = _copyablePlainText().trim().isNotEmpty;

    return Semantics(
      container: true,
      label: 'Ответ ассистента с вызовами инструментов',
      child: Align(
        alignment: Alignment.centerLeft,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            ClipRRect(
              borderRadius: const BorderRadius.only(
                topLeft: Radius.circular(20),
                topRight: Radius.circular(20),
                bottomLeft: Radius.circular(8),
                bottomRight: Radius.circular(20),
              ),
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
                  children: [
                    if (tail != null) ...[
                      if (storedReasoning.isNotEmpty)
                        Theme(
                          data: theme,
                          child: ExpansionTile(
                            tilePadding: EdgeInsets.zero,
                            childrenPadding: const EdgeInsets.only(bottom: 8),
                            title: Text('Размышление', style: tileTitleStyle),
                            children: [
                              SelectableText(
                                storedReasoning,
                                style: monoReason,
                              ),
                            ],
                          ),
                        ),
                      if (tagThinkingTrim.isNotEmpty)
                        Theme(
                          data: theme,
                          child: ExpansionTile(
                            tilePadding: EdgeInsets.zero,
                            childrenPadding: const EdgeInsets.only(bottom: 8),
                            title: Text(
                              'Размышление модели',
                              style: tileTitleStyle,
                            ),
                            children: [
                              SelectableText(
                                tagThinkingTrim,
                                style: monoReason,
                              ),
                            ],
                          ),
                        ),
                    ],
                    Theme(
                      data: theme,
                      child: ExpansionTile(
                        tilePadding: EdgeInsets.zero,
                        childrenPadding: const EdgeInsets.only(bottom: 8, top: 4),
                        title: Text(
                          'Инструменты ($_totalTools)',
                          style: tileTitleStyle,
                        ),
                        children: [
                          for (var si = 0; si < widget.segments.length; si++) ...[
                            if (si > 0) const SizedBox(height: 14),
                            if (widget.segments.length > 1)
                              Padding(
                                padding: const EdgeInsets.only(bottom: 8),
                                child: Text(
                                  'Шаг ${si + 1}',
                                  style: TextStyle(
                                    fontSize: 11,
                                    fontWeight: FontWeight.w600,
                                    color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.85),
                                  ),
                                ),
                              ),
                            Builder(
                              builder: (context) {
                                final seg = widget.segments[si];
                                final names = parseToolCallNamesFromToolCallsJson(
                                  seg.leadAssistant.toolCallsJson,
                                );
                                return Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    for (
                                      var i = 0;
                                      i < seg.toolMessages.length;
                                      i++
                                    ) ...[
                                      if (i > 0) const SizedBox(height: 12),
                                      Align(
                                        alignment: Alignment.centerLeft,
                                        child: Text(
                                          (seg.toolMessages[i].toolName?.trim().isNotEmpty ??false)
                                              ? seg.toolMessages[i].toolName!.trim()
                                              : (i < names.length
                                                    ? names[i]
                                                    : 'Вызов ${i + 1}'),
                                          style: TextStyle(
                                            fontSize: 12,
                                            fontWeight: FontWeight.w600,
                                            color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.9),
                                          ),
                                        ),
                                      ),
                                      const SizedBox(height: 4),
                                      SelectableText(
                                        formatToolResultForUser(seg.toolMessages[i].content),
                                        style: TextStyle(
                                          fontSize: 14,
                                          height: 1.45,
                                          color: messageTextColor,
                                        ),
                                      ),
                                    ],
                                  ],
                                );
                              },
                            ),
                          ],
                        ],
                      ),
                    ),
                    const SizedBox(height: 8),
                    Theme(
                      data: theme,
                      child: ExpansionTile(
                        tilePadding: EdgeInsets.zero,
                        childrenPadding: const EdgeInsets.only(top: 8),
                        title: Text(
                          'Размещение',
                          style: tileTitleStyle,
                        ),
                        children: [
                          SelectableText(
                            _placementBody(),
                            style: TextStyle(
                              fontSize: 11,
                              height: 1.35,
                              fontFamily: 'monospace',
                              color: messageTextColor.withValues(alpha: 0.88),
                            ),
                          ),
                        ],
                      ),
                    ),
                    if (tail != null) ...[
                      const SizedBox(height: 16),
                      Divider(
                        height: 1,
                        color: theme.colorScheme.outlineVariant.withValues(alpha: 0.6),
                      ),
                      const SizedBox(height: 12),
                      Text(
                        'Ответ',
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                          color: messageTextColor,
                        ),
                      ),
                      const SizedBox(height: 8),
                      if (hasMainText)
                        buildAssistantMarkdownFromContent(
                          context,
                          tailBody,
                          enableMarkdownParseGuard: true,
                        )
                      else if (storedReasoning.isEmpty && tagThinkingTrim.isEmpty)
                        Text(
                          'Итоговый текст появится после обработки инструментов.',
                          style: TextStyle(
                            fontSize: 13,
                            height: 1.45,
                            color: theme.colorScheme.onSurfaceVariant,
                            fontStyle: FontStyle.italic,
                          ),
                        ),
                    ] else ...[
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(
                          'Ожидаю итоговый ответ модели…',
                          style: TextStyle(
                            fontSize: 13,
                            color: theme.colorScheme.onSurfaceVariant,
                            fontStyle: FontStyle.italic,
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            if (hasCopyable || widget.onRegenerate != null || widget.showContinuePartial || widget.showEditNav)
              Padding(
                padding: const EdgeInsets.only(left: 4, right: 4, top: 2, bottom: 4),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (widget.showEditNav) ...[
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
                        label: 'Версия сообщения ${(widget.editsIndex ?? 0) + 1} из ${widget.editsTotal ?? 1}',
                        child: Text(
                          '${(widget.editsIndex ?? 0) + 1}/${widget.editsTotal ?? 1}',
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
                    if (hasCopyable)
                      Semantics(
                        excludeSemantics: true,
                        label: _justCopied
                            ? 'Текст скопирован'
                            : 'Копировать сводку и ответ',
                        button: true,
                        child: IconButton(
                          onPressed: () async {
                            await Clipboard.setData(
                              ClipboardData(text: _copyablePlainText()),
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
                          },
                          icon: Icon(
                            _justCopied
                                ? Icons.check_rounded
                                : Icons.copy_rounded,
                            size: 18,
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                          tooltip: _justCopied ? 'Скопировано' : 'Копировать',
                          padding: EdgeInsets.zero,
                          visualDensity: VisualDensity.compact,
                          constraints: const BoxConstraints(
                            minWidth: 32,
                            minHeight: 32,
                          ),
                        ),
                      ),
                    if (widget.showContinuePartial && widget.onContinueAssistant != null) ...[
                      const SizedBox(width: 4),
                      Semantics(
                        excludeSemantics: true,
                        label: 'Продолжить ответ ассистента',
                        button: true,
                        child: IconButton(
                          onPressed: widget.onContinueAssistant,
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
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}
