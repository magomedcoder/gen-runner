import 'package:flutter/material.dart';

class SessionsListHeader extends StatelessWidget {
  const SessionsListHeader({
    super.key,
    required this.onToggleCollapse,
  });

  final VoidCallback onToggleCollapse;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Material(
      color: theme.colorScheme.surfaceContainerLow,
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
        decoration: BoxDecoration(
          border: Border(
            bottom: BorderSide(
              color: theme.dividerColor.withValues(alpha: 0.12),
            ),
          ),
        ),
        child: Row(
          children: [
            Expanded(
              child: Text(
                'Чаты',
                style: theme.textTheme.titleSmall?.copyWith(
                  fontWeight: FontWeight.w600,
                  letterSpacing: 0.2,
                ),
              ),
            ),
            IconButton(
              icon: Icon(
                Icons.menu_open_rounded,
                color: theme.colorScheme.onSurfaceVariant,
              ),
              onPressed: onToggleCollapse,
              tooltip: 'Скрыть список чатов',
            ),
          ],
        ),
      ),
    );
  }
}
