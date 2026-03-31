import 'package:flutter_test/flutter_test.dart';
import 'package:gen/core/session_file_id_scan.dart';

void main() {
  group('extractSessionFileIdsFromText', () {
    test('empty and no matches', () {
      expect(extractSessionFileIdsFromText(''), isEmpty);
      expect(extractSessionFileIdsFromText('здесь нет идентификаторов'), isEmpty);
    });

    test('file_id and workbook_file_id', () {
      expect(
        extractSessionFileIdsFromText(
          '{"file_id": 10, "ok": true}',
        ),
        [10],
      );
      expect(
        extractSessionFileIdsFromText(
          r'"workbook_file_id": 99',
        ),
        [99],
      );
    });

    test('preserves order, deduplicates', () {
      const s = '''
{"file_id":1}
later "workbook_file_id" : 2
"file_id": 1 again
''';
      expect(extractSessionFileIdsFromText(s), [1, 2]);
    });

    test('skips zero and negative-looking tokens (only positive digits)', () {
      expect(extractSessionFileIdsFromText('"file_id": 0'), isEmpty);
      expect(extractSessionFileIdsFromText('"file_id": 42'), [42]);
    });

    test('caps at 16 unique ids', () {
      final buf = StringBuffer();
      for (var i = 1; i <= 20; i++) {
        buf.writeln('"file_id": $i');
      }
      final got = extractSessionFileIdsFromText(buf.toString());
      expect(got.length, 16);
      expect(got.first, 1);
      expect(got.last, 16);
    });

    test('multiline JSON blob', () {
      const s = '''
Результат:
{
  "preview_tsv": "я\\tб",
  "workbook_file_id":  12345
}
''';
      expect(extractSessionFileIdsFromText(s), [12345]);
    });
  });
}
