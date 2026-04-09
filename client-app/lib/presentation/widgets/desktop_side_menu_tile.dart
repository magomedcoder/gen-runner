import 'package:flutter/material.dart';

class DesktopSideMenuTile extends StatelessWidget {
  const DesktopSideMenuTile({
    super.key,
    required this.icon,
    required this.title,
    required this.selected,
    required this.onTap,
  });

  final IconData icon;
  final String title;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final selectedBg = theme.colorScheme.primaryContainer;
    final selectedFg = theme.colorScheme.onPrimaryContainer;
    final normalFg = theme.colorScheme.onSurfaceVariant;

    return Material(
      color: selected ? selectedBg : Colors.transparent,
      borderRadius: BorderRadius.circular(14),
      child: InkWell(
        borderRadius: BorderRadius.circular(14),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          child: Row(
            children: [
              Icon(
                icon,
                color: selected ? selectedFg : normalFg,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  title,
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                    color: selected ? selectedFg : null,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
