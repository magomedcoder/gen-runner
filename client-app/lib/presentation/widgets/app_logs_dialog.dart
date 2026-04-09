import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:gen/core/log/logs.dart';

String _levelLetter(Level level) {
  return switch (level) {
    Level.debug => 'D',
    Level.verbose => 'V',
    Level.info => 'I',
    Level.warn => 'W',
    Level.error => 'E',
  };
}

String _formatTime(DateTime t) {
  String two(int n) => n.toString().padLeft(2, '0');
  String three(int n) => n.toString().padLeft(3, '0');
  return '${two(t.hour)}:${two(t.minute)}:${two(t.second)}.${three(t.millisecond)}';
}

Future<void> showAppLogsDialog(BuildContext context) {
  Logs().i('Журнал приложения открыт: экран «О приложении»');
  final events = Logs().events;
  final scaffoldMessenger = ScaffoldMessenger.maybeOf(context);

  return showDialog<void>(
    context: context,
    builder: (ctx) {
      final scheme = Theme.of(ctx).colorScheme;
      final textTheme = Theme.of(ctx).textTheme;

      Color levelColor(Level l) {
        return switch (l) {
          Level.debug => scheme.outline,
          Level.verbose => scheme.onSurfaceVariant,
          Level.info => scheme.primary,
          Level.warn => scheme.tertiary,
          Level.error => scheme.error,
        };
      }

      final buffer = StringBuffer();
      for (final e in events) {
        buffer.writeln('${_formatTime(e.time)} ${_levelLetter(e.level)} ${e.message}');
        if (e.exception != null) {
          buffer.writeln('  ex: ${e.exception}');
        }
      }
      final allText = buffer.toString();

      return AlertDialog(
        title: const Text('Журнал'),
        content: SizedBox(
          width: double.maxFinite,
          child: events.isEmpty
            ? Text(
              'Записей пока нет',
              style: textTheme.bodyMedium?.copyWith(color: scheme.onSurfaceVariant),
            )
            : ConstrainedBox(
              constraints: BoxConstraints(maxHeight: MediaQuery.sizeOf(ctx).height * 0.55),
              child: ListView.builder(
                shrinkWrap: true,
                itemCount: events.length,
                itemBuilder: (context, index) {
                  final e = events[index];
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 8),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text.rich(
                          TextSpan(
                            children: [
                              TextSpan(
                                text: '${_formatTime(e.time)} ',
                                style: textTheme.bodySmall?.copyWith(
                                  color: scheme.onSurfaceVariant,
                                  fontFamily: 'monospace',
                                  fontSize: 11,
                                ),
                              ),
                              TextSpan(
                                text: '${_levelLetter(e.level)} ',
                                style: textTheme.bodySmall?.copyWith(
                                  color: levelColor(e.level),
                                  fontWeight: FontWeight.w700,
                                  fontFamily: 'monospace',
                                  fontSize: 11,
                                ),
                              ),
                              TextSpan(
                                text: e.message,
                                style: textTheme.bodySmall?.copyWith(
                                  height: 1.25,
                                ),
                              ),
                            ],
                          ),
                        ),
                        if (e.exception != null)
                          Padding(
                            padding: const EdgeInsets.only(top: 2, left: 4),
                            child: Text(
                              e.exception.toString(),
                              style: textTheme.bodySmall?.copyWith(
                                color: scheme.error,
                                fontSize: 11,
                                height: 1.2,
                              ),
                            ),
                          ),
                      ],
                    ),
                  );
                  },
                ),
            ),
        ),
        actions: [
          if (events.isNotEmpty)
            TextButton(
              onPressed: () async {
                await Clipboard.setData(ClipboardData(text: allText));
                scaffoldMessenger?.showSnackBar(
                  const SnackBar(content: Text('Скопировано в буфер')),
                );
              },
              child: const Text('Копировать всё'),
            ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('Закрыть'),
          ),
        ],
      );
    },
  );
}
