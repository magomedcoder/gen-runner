import 'dart:convert';

List<String> parseToolCallNamesFromToolCallsJson(String? toolCallsJson) {
  if (toolCallsJson == null || toolCallsJson.trim().isEmpty) {
    return const [];
  }

  try {
    final decoded = json.decode(toolCallsJson);
    if (decoded is! List) {
      return const [];
    }

    final out = <String>[];
    for (final e in decoded) {
      if (e is! Map) {
        continue;
      }

      final fn = e['function'];
      if (fn is Map && fn['name'] is String) {
        final n = (fn['name'] as String).trim();
        if (n.isNotEmpty) {
          out.add(n);
        }

        continue;
      }

      if (e['tool_name'] is String) {
        final n = (e['tool_name'] as String).trim();
        if (n.isNotEmpty) {
          out.add(n);
        }
      }
    }

    return out;
  } catch (_) {
    return const [];
  }
}

String formatToolResultForUser(String raw) {
  final s = raw.trim();
  if (s.isEmpty) {
    return '—';
  }

  try {
    final d = json.decode(s);
    if (d is Map<String, dynamic>) {
      if (d['ok'] == true && d['text'] is String) {
        return (d['text'] as String).trim();
      }

      if (d['text'] is String && (d['text'] as String).trim().isNotEmpty) {
        return (d['text'] as String).trim();
      }

      if (d['message'] is String) {
        return (d['message'] as String).trim();
      }

      if (d['answer'] is String) {
        return (d['answer'] as String).trim();
      }

      if (d['error'] != null) {
        return 'Ошибка: ${d['error']}';
      }

      if (d['content'] is String && (d['content'] as String).trim().isNotEmpty) {
        return (d['content'] as String).trim();
      }

      if (d['result'] is String) {
        return (d['result'] as String).trim();
      }

      return 'Получен ответ инструмента (${d.length} полей). Подробности — в разделе «Размещение».';
    }

    if (d is List) {
      return 'Получен список из ${d.length} элементов. Подробности — в разделе «Размещение».';
    }
  } catch (_) {

  }

  if (s.length > 4000) {
    return '${s.substring(0, 4000)}…';
  }

  return s;
}
