class RedactedThinkingSplit {
  RedactedThinkingSplit._();

  static const String _openTag = '\u003Credacted_thinking\u003E';
  static const String _closeTag = '\u003C/redacted_thinking\u003E';

  static int _indexOfIgnoreCase(String haystack, String needle, [int start = 0]) {
    if (needle.isEmpty || start >= haystack.length) {
      return -1;
    }
    final h = haystack.toLowerCase();
    final n = needle.toLowerCase();
    return h.indexOf(n, start);
  }

  static (String visible, String? tagReasoning) peel(String source) {
    if (source.isEmpty) {
      return ('', null);
    }
    var rest = source;
    final out = StringBuffer();
    final thinking = StringBuffer();
    while (true) {
      final openIdx = _indexOfIgnoreCase(rest, _openTag);
      if (openIdx < 0) {
        out.write(rest);
        break;
      }
      out.write(rest.substring(0, openIdx));
      final afterOpenStart = openIdx + _openTag.length;
      if (afterOpenStart > rest.length) {
        break;
      }
      final tail = rest.substring(afterOpenStart);
      final closeIdx = _indexOfIgnoreCase(tail, _closeTag);
      if (closeIdx < 0) {
        if (tail.trim().isNotEmpty) {
          if (thinking.isNotEmpty) {
            thinking.writeln();
          }
          thinking.write(tail);
        }
        break;
      }
      final inner = tail.substring(0, closeIdx);
      if (inner.trim().isNotEmpty) {
        if (thinking.isNotEmpty) {
          thinking.writeln();
        }
        thinking.write(inner);
      }
      rest = tail.substring(closeIdx + _closeTag.length);
    }
    final t = thinking.toString().trim();
    return (out.toString(), t.isEmpty ? null : t);
  }

  static String combine(String? nativeReasoning, String? tagReasoning) {
    final a = nativeReasoning?.trim() ?? '';
    final b = tagReasoning?.trim() ?? '';
    if (a.isEmpty) {
      return b;
    }
    if (b.isEmpty) {
      return a;
    }
    return '$a\n\n$b';
  }

  static bool hasAssistantPayload(String rawText, String nativeReasoning) {
    if (nativeReasoning.trim().isNotEmpty) {
      return true;
    }
    final p = peel(rawText);
    return p.$1.trim().isNotEmpty || (p.$2 ?? '').trim().isNotEmpty;
  }
}
