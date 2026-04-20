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

    test('think tags (DeepSeek-style)', () {
      final r = RedactedThinkingSplit.peel(
        'Ans\u003Cthink\u003Estep\u003C/think\u003Ewer',
      );
      expect(r.$1, 'Answer');
      expect(r.$2, 'step');
    });

    test('reasoning tags', () {
      final r = RedactedThinkingSplit.peel(
        'out\u003Creasoning\u003Er\u003C/reasoning\u003Ez',
      );
      expect(r.$1, 'outz');
      expect(r.$2, 'r');
    });

    test('earliest tag wins when both present', () {
      final r = RedactedThinkingSplit.peel(
        'a\u003Cthink\u003Et\u003C/think\u003Eb\u003Credacted_thinking\u003Eu\u003C/redacted_thinking\u003Ev',
      );
      expect(r.$1, 'abv');
      expect(r.$2, 't\nu');
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
