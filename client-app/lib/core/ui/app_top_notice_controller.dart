import 'dart:async';

import 'package:flutter/foundation.dart';

class AppTopNoticeEntry {
  AppTopNoticeEntry({
    required this.id,
    required this.message,
    required this.error,
    required this.duration,
  });

  final int id;
  final String message;
  final bool error;
  final Duration duration;
}

class AppTopNoticeController extends ChangeNotifier {
  AppTopNoticeEntry? _current;
  final List<AppTopNoticeEntry> _queue = [];
  Timer? _timer;
  int _nextId = 0;

  AppTopNoticeEntry? get current => _current;

  void show(
    String message, {
    bool error = false,
    Duration? duration,
  }) {
    final d = duration ?? Duration(seconds: error ? 5 : 4);
    final entry = AppTopNoticeEntry(
      id: _nextId++,
      message: message,
      error: error,
      duration: d,
    );
    if (_current == null) {
      _activate(entry);
    } else {
      _queue.add(entry);
    }
  }

  void _activate(AppTopNoticeEntry entry) {
    _timer?.cancel();
    _current = entry;
    notifyListeners();
    _timer = Timer(entry.duration, _onTimer);
  }

  void _onTimer() {
    _timer?.cancel();
    _timer = null;
    if (_queue.isNotEmpty) {
      _activate(_queue.removeAt(0));
    } else {
      _current = null;
      notifyListeners();
    }
  }

  void dismissCurrent() {
    _timer?.cancel();
    _timer = null;
    if (_queue.isNotEmpty) {
      _activate(_queue.removeAt(0));
    } else {
      _current = null;
      notifyListeners();
    }
  }
}
