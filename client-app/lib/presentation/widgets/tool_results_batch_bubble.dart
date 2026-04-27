import 'package:flutter/material.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/core/tool_display_format.dart';
import 'package:gen/domain/entities/message.dart';

class ToolResultsBatchBubble extends StatelessWidget {
  const ToolResultsBatchBubble({super.key, required this.tools});

  final List<Message> tools;

  String _placementRaw() {
    final b = StringBuffer();
    for (var i = 0; i < tools.length; i++) {
      if (i > 0) {
        b.writeln();
      }

      b.writeln('--- ${i + 1} (${tools[i].toolName ?? tools[i].toolCallId ?? ''}) ---');
      b.write(tools[i].content.trim());
    }
    return b.toString();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final width = Breakpoints.width(context);
    const minBubbleWidth = 64.0;
    final maxBubbleWidth = Breakpoints.isMobile(context)
    ? width * 0.88
    : (Breakpoints.isTablet(context) ? 420.0 : 560.0);
    final messageTextColor = theme.colorScheme.onSurface;

    return Semantics(
      container: true,
      label: 'Результаты инструментов, ${tools.length} вызовов',
      child: Align(
        alignment: Alignment.centerLeft,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            ClipRRect(
              borderRadius: BorderRadius.circular(16),
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
                    Theme(
                      data: theme,
                      child: ExpansionTile(
                        tilePadding: EdgeInsets.zero,
                        childrenPadding: const EdgeInsets.only(bottom: 8, top: 4),
                        title: Text(
                          'Инструменты (${tools.length})',
                          style: TextStyle(
                            fontSize: 12,
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                        ),
                        children: [
                          for (var i = 0; i < tools.length; i++) ...[
                            if (i > 0)
                              Padding(
                                padding: const EdgeInsets.symmetric(vertical: 10),
                                child: Divider(
                                  height: 1,
                                  color: theme.colorScheme.outlineVariant
                                      .withValues(alpha: 0.5),
                                ),
                              ),
                            if (tools[i].toolName != null &&
                                tools[i].toolName!.trim().isNotEmpty)
                              Padding(
                                padding: const EdgeInsets.only(bottom: 6),
                                child: Text(
                                  tools[i].toolName!.trim(),
                                  style: TextStyle(
                                    fontSize: 11,
                                    color: theme.colorScheme.onSurfaceVariant
                                        .withValues(alpha: 0.75),
                                    fontStyle: FontStyle.italic,
                                  ),
                                ),
                              ),
                            SelectableText(
                              formatToolResultForUser(tools[i].content),
                              style: TextStyle(
                                color: messageTextColor,
                                fontSize: 14,
                                height: 1.45,
                              ),
                            ),
                          ],
                        ],
                      ),
                    ),
                    Theme(
                      data: theme,
                      child: ExpansionTile(
                        tilePadding: EdgeInsets.zero,
                        childrenPadding: const EdgeInsets.only(top: 4),
                        title: Text(
                          'Размещение',
                          style: TextStyle(
                            fontSize: 12,
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                        ),
                        children: [
                          SelectableText(
                            _placementRaw(),
                            style: TextStyle(
                              color: messageTextColor.withValues(alpha: 0.88),
                              fontSize: 11,
                              height: 1.35,
                              fontFamily: 'monospace',
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
