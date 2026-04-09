import 'package:flutter/material.dart';
import 'package:gen/domain/entities/rag_ingestion_ui.dart';

class RagIngestionStatusBanner extends StatelessWidget {
  const RagIngestionStatusBanner({super.key, required this.ui});

  final RagIngestionUi ui;

  static String _ellipsize(String name, {int max = 42}) {
    final t = name.trim();
    if (t.length <= max) {
      return t;
    }
    return '${t.substring(0, max - 1)}...';
  }

  String _title() {
    switch (ui.phase) {
      case RagIngestionPhase.uploadingFile:
        return 'Загрузка файла...';
      case RagIngestionPhase.queued:
        return 'Файл в очереди на индексацию';
      case RagIngestionPhase.indexing:
        return 'Индексация документа...';
      case RagIngestionPhase.ready:
        return 'Индекс готов - поиск по документу включён';
      case RagIngestionPhase.willSendWithoutRag:
        return 'Сообщение уйдёт без поиска по документу';
    }
  }

  String? _subtitle() {
    final name = _ellipsize(ui.fileName);
    final chunks = ui.chunkCount > 0 ? ' ${ui.chunkCount} фрагм.' : '';
    switch (ui.phase) {
      case RagIngestionPhase.uploadingFile:
      case RagIngestionPhase.queued:
      case RagIngestionPhase.indexing:
        return '$name$chunks';
      case RagIngestionPhase.ready:
        return '$name$chunks';
      case RagIngestionPhase.willSendWithoutRag:
        final d = ui.detail.trim();
        if (d.isEmpty) {
          return name;
        }
        return '$name\n$d';
    }
  }

  (Color bg, Color fg, IconData icon) _colorsAndIcon(ColorScheme cs) {
    switch (ui.phase) {
      case RagIngestionPhase.uploadingFile:
      case RagIngestionPhase.queued:
      case RagIngestionPhase.indexing:
        return (
          cs.primaryContainer,
          cs.onPrimaryContainer,
          Icons.hourglass_top_rounded,
        );
      case RagIngestionPhase.ready:
        return (
          cs.tertiaryContainer,
          cs.onTertiaryContainer,
          Icons.check_circle_outline_rounded,
        );
      case RagIngestionPhase.willSendWithoutRag:
        return (
          cs.errorContainer,
          cs.onErrorContainer,
          Icons.warning_amber_rounded,
        );
    }
  }

  bool get _showProgress {
    switch (ui.phase) {
      case RagIngestionPhase.uploadingFile:
      case RagIngestionPhase.queued:
      case RagIngestionPhase.indexing:
        return true;
      case RagIngestionPhase.ready:
      case RagIngestionPhase.willSendWithoutRag:
        return false;
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final (bg, fg, icon) = _colorsAndIcon(cs);
    final subtitle = _subtitle();

    return Material(
      color: bg,
      elevation: 1,
      shadowColor: cs.shadow.withValues(alpha: 0.12),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(icon, size: 22, color: fg),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        _title(),
                        style: theme.textTheme.titleSmall?.copyWith(
                          color: fg,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                      if (subtitle != null && subtitle.isNotEmpty) ...[
                        const SizedBox(height: 4),
                        Text(
                          subtitle,
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: fg.withValues(alpha: 0.92),
                            height: 1.35,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ],
            ),
            if (_showProgress) ...[
              const SizedBox(height: 10),
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: LinearProgressIndicator(
                  minHeight: 3,
                  backgroundColor: fg.withValues(alpha: 0.2),
                  valueColor: AlwaysStoppedAnimation<Color>(fg),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
