import 'package:flutter/material.dart';

class SessionsDrawerHeader extends StatelessWidget {
  const SessionsDrawerHeader({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 12, 8, 12),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(
            color: Theme.of(context).dividerColor.withValues(alpha: 0.1),
          ),
        ),
      ),
      child: Row(
        children: [
          Text(
            'Чаты',
            style: Theme.of(
              context,
            ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600),
          ),
          const Spacer(),
          IconButton(
            icon: const Icon(Icons.close),
            onPressed: () => Navigator.of(context).pop(),
            tooltip: 'Закрыть',
          ),
        ],
      ),
    );
  }
}
