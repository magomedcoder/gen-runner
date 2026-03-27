import 'package:flutter/material.dart';

class ChatDropOverlay extends StatelessWidget {
  const ChatDropOverlay({super.key});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Positioned.fill(
      child: IgnorePointer(
        child: Container(
          margin: const EdgeInsets.only(bottom: 1),
          decoration: BoxDecoration(
            color: theme.colorScheme.primaryContainer.withValues(alpha: 0.25),
            border: Border.all(
              color: theme.colorScheme.primary.withValues(alpha: 0.5),
              width: 2,
              strokeAlign: BorderSide.strokeAlignInside,
            ),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.upload_file_rounded,
                  size: 48,
                ),
                const SizedBox(height: 8),
                Text(
                  'Отпустите файл, чтобы прикрепить',
                  style: theme.textTheme.titleMedium?.copyWith(
                    color: theme.colorScheme.onSurface,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
