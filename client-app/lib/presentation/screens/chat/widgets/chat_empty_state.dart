import 'package:flutter/material.dart';
import 'package:gen/core/layout/responsive.dart';

class ChatEmptyState extends StatelessWidget {
  const ChatEmptyState({super.key});

  @override
  Widget build(BuildContext context) {
    final useDrawer = Breakpoints.useDrawerForSessions(context);
    return Center(
      child: Padding(
        padding: EdgeInsets.symmetric(
          horizontal: Breakpoints.isMobile(context) ? 24 : 32,
          vertical: 32,
        ),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            SizedBox(
              width: 120,
              height: 120,
              child: Icon(
                Icons.chat_bubble_outline,
                size: 54,
              ),
            ),
            const SizedBox(height: 24),
            Text(
              useDrawer
                  ? 'Нажмите ☰ чтобы выбрать сессию\nили создайте новую'
                  : 'Выберите сессию из списка слева\nили создайте новую',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
