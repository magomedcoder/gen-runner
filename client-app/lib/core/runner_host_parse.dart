class ParsedRunnerHost {
  final String host;
  final int port;

  const ParsedRunnerHost({required this.host, required this.port});
}

ParsedRunnerHost? parseRunnerHostInput(String input, int fallbackPort) {
  final s = input.trim();
  if (s.isEmpty) {
    return null;
  }

  if (s.startsWith('[')) {
    final bracketPort = s.indexOf(']:');
    if (bracketPort != -1) {
      final inside = s.substring(1, bracketPort);
      final p = int.tryParse(s.substring(bracketPort + 2).trim());
      if (p != null && p > 0 && p <= 65535 && inside.isNotEmpty) {
        return ParsedRunnerHost(host: inside, port: p);
      }

      return null;
    }

    if (s.endsWith(']') && s.length > 2) {
      final inside = s.substring(1, s.length - 1);
      if (inside.isNotEmpty && fallbackPort > 0 && fallbackPort <= 65535) {
        return ParsedRunnerHost(host: inside, port: fallbackPort);
      }
    }

    return null;
  }

  final lastColon = s.lastIndexOf(':');
  if (lastColon > 0 && lastColon < s.length - 1) {
    final left = s.substring(0, lastColon);
    var right = s.substring(lastColon + 1);
    final zone = right.indexOf('%');
    if (zone != -1) {
      right = right.substring(0, zone);
    }

    if (RegExp(r'^\d{1,5}$').hasMatch(right) && !left.contains(':')) {
      final p = int.tryParse(right);
      if (p != null && p > 0 && p <= 65535) {
        return ParsedRunnerHost(host: left, port: p);
      }
    }
  }

  if (fallbackPort <= 0 || fallbackPort > 65535) return null;
  return ParsedRunnerHost(host: s, port: fallbackPort);
}
