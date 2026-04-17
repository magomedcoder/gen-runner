import 'package:flutter_test/flutter_test.dart';
import 'package:gen/core/redacted_thinking_split.dart';

void main() {
  group('RedactedThinkingSplit.peel', () {
    test('no tags returns source', () {
      final r = RedactedThinkingSplit.peel('Hello **world**');
      expect(r.$1, 'Hello **world**');
      expect(r.$2, isNull);
    });

    test('single block', () {
      final r = RedactedThinkingSplit.peel(
        'Hi\u003Credacted_thinking\u003Eplan A\u003C/redacted_thinking\u003Ethere',
      );
      expect(r.$1, 'Hithere');
      expect(r.$2, 'plan A');
    });

    test('case insensitive tags', () {
      final r = RedactedThinkingSplit.peel(
        'x\u003CREDACTED_THINKING\u003Einner\u003C/Redacted_Thinking\u003Ey',
      );
      expect(r.$1, 'xy');
      expect(r.$2, 'inner');
    });

    test('unclosed block keeps tail as reasoning', () {
      final r = RedactedThinkingSplit.peel(
        'a\u003Credacted_thinking\u003Etail',
      );
      expect(r.$1, 'a');
      expect(r.$2, 'tail');
    });

    test('combine orders native then tags', () {
      expect(
        RedactedThinkingSplit.combine('n', 't'),
        'n\n\nt',
      );
    });
  });
}
