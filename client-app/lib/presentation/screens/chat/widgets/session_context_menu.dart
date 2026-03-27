import 'package:flutter/material.dart';

class SessionContextMenu {
  SessionContextMenu._();

  static Future<void> showDesktopMenu(
    BuildContext context, {
    required RelativeRect position,
    required VoidCallback onEdit,
    required VoidCallback onDelete,
  }) {
    return showMenu<String>(
      context: context,
      position: position,
      items: [
        PopupMenuItem<String>(
          value: 'edit',
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.edit,
                size: 20,
              ),
              const SizedBox(width: 12),
              const Text('Редактировать название'),
            ],
          ),
        ),
        PopupMenuItem<String>(
          value: 'delete',
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.delete_outline,
                size: 20,
              ),
              const SizedBox(width: 12),
              Text(
                'Удалить',
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ],
          ),
        ),
      ],
    ).then((value) {
      if (value == 'edit') {
        onEdit();
      }

      if (value == 'delete') {
        onDelete();
      }
    });
  }

  static Future<void> showMobileSheet(
    BuildContext context, {
    required VoidCallback onEdit,
    required VoidCallback onDelete,
  }) {
    return showModalBottomSheet<void>(
      context: context,
      backgroundColor: Colors.transparent,
      builder: (sheetContext) => Container(
        margin: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: Theme.of(sheetContext).colorScheme.surface,
          borderRadius: BorderRadius.circular(16),
          boxShadow: [
            BoxShadow(
              color: Theme.of(sheetContext).colorScheme.shadow.withValues(alpha: 0.35),
              blurRadius: 20,
              spreadRadius: 2,
            ),
          ],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: Icon(Icons.edit),
              title: const Text('Редактировать название'),
              onTap: () {
                Navigator.pop(sheetContext);
                onEdit();
              },
            ),
            const Divider(height: 1),
            ListTile(
              leading: Icon(Icons.delete_outline),
              title: Text(
                'Удалить',
                style: TextStyle(
                  color: Theme.of(sheetContext).colorScheme.error,
                ),
              ),
              onTap: () {
                Navigator.pop(sheetContext);
                onDelete();
              },
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}
