import 'package:flutter/material.dart';
import 'package:gen/core/attachment_settings.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';

Future<void> showDeleteSessionDialog(
  BuildContext context, {
  required int sessionId,
  required String sessionTitle,
  required ChatBloc chatBloc,
}) {
  return showDialog<void>(
    context: context,
    builder: (dialogContext) => AlertDialog(
      title: const Text('Удалить сессию?'),
      content: Text('Вы уверены, что хотите удалить сессию "$sessionTitle"?'),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(dialogContext).pop(),
          child: const Text('Отмена'),
        ),
        TextButton(
          onPressed: () {
            chatBloc.add(ChatDeleteSession(sessionId));
            Navigator.of(dialogContext).pop();
          },
          child: const Text('Удалить', style: TextStyle(color: Colors.red)),
        ),
      ],
    ),
  );
}

void showSupportedFormatsDialog(BuildContext context) {
  final theme = Theme.of(context);
  final isMobile = Breakpoints.isMobile(context);
  final maxWidth = isMobile ? MediaQuery.sizeOf(context).width - 32 : 400.0;

  showDialog<void>(
    context: context,
    builder: (dialogContext) => AlertDialog(
      insetPadding: EdgeInsets.symmetric(
        horizontal: isMobile ? 16 : 40,
        vertical: 24,
      ),
      contentPadding: const EdgeInsets.fromLTRB(24, 20, 24, 0),
      title: Row(
        children: [
          Icon(
            Icons.insert_drive_file_outlined,
            color: theme.colorScheme.primary,
            size: isMobile ? 22 : 24,
          ),
          SizedBox(width: isMobile ? 8 : 10),
          Flexible(
            child: Text(
              'Поддерживаемые форматы',
              style: theme.textTheme.titleMedium?.copyWith(
                fontWeight: FontWeight.w600,
              ),
              overflow: TextOverflow.ellipsis,
              maxLines: 2,
            ),
          ),
        ],
      ),
      content: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Текст',
                style: theme.textTheme.labelMedium?.copyWith(
                  color: theme.colorScheme.primary,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                AttachmentSettings.textFormatLabels.join(', '),
                style: theme.textTheme.bodyMedium,
              ),
              const SizedBox(height: 12),
              Text(
                'Документы',
                style: theme.textTheme.labelMedium?.copyWith(
                  color: theme.colorScheme.primary,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                AttachmentSettings.documentFormatLabels.join(', '),
                style: theme.textTheme.bodyMedium,
              ),
              const SizedBox(height: 16),
              Text(
                'Макс. размер: ${AttachmentSettings.maxFileSizeKb} КБ',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(dialogContext).pop(),
          child: const Text('Закрыть'),
        ),
      ],
    ),
  );
}
