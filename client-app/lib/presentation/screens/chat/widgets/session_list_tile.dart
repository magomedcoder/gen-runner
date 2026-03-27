import 'package:flutter/material.dart';
import 'package:gen/domain/entities/session.dart';

class SessionListTile extends StatelessWidget {
  const SessionListTile({
    super.key,
    required this.session,
    required this.isSelected,
    required this.isDesktop,
    required this.onTap,
    required this.onLongPress,
    this.onSecondaryTapDown,
  });

  final ChatSession session;
  final bool isSelected;
  final bool isDesktop;
  final VoidCallback onTap;
  final VoidCallback onLongPress;
  final void Function(TapDownDetails details)? onSecondaryTapDown;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: isSelected
            ? Theme.of(context).colorScheme.primaryContainer
            : Colors.transparent,
        borderRadius: BorderRadius.circular(12),
        border: isSelected
            ? Border.all(
                color: Theme.of(
                  context,
                ).colorScheme.primary.withValues(alpha: 0.3),
                width: 1,
              )
            : null,
      ),
      child: Material(
        color: Colors.transparent,
        child: GestureDetector(
          onSecondaryTapDown: isDesktop ? onSecondaryTapDown : null,
          child: InkWell(
            borderRadius: BorderRadius.circular(12),
            onTap: onTap,
            onLongPress: onLongPress,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Text(
                          session.title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: isSelected
                                ? FontWeight.w600
                                : FontWeight.normal,
                            color: isSelected
                                ? Theme.of(
                                    context,
                                  ).colorScheme.onPrimaryContainer
                                : Theme.of(context).colorScheme.onSurface,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (isSelected)
                    Icon(
                      Icons.chevron_right,
                      size: 18,
                    ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
