import 'package:flutter_test/flutter_test.dart';
import 'package:gen/presentation/screens/chat/bloc/session_file_rag_wait.dart';

void main() {
  group('filenameEligibleForSessionRag', () {
    test('allows known document extensions', () {
      expect(filenameEligibleForSessionRag('Doc.PDF'), isTrue);
      expect(filenameEligibleForSessionRag('a.docx'), isTrue);
      expect(filenameEligibleForSessionRag('notes.md'), isTrue);
      expect(filenameEligibleForSessionRag('data.csv'), isTrue);
      expect(filenameEligibleForSessionRag('sheet.xlsx'), isTrue);
    });

    test('rejects non-rag extensions', () {
      expect(filenameEligibleForSessionRag('pic.png'), isFalse);
      expect(filenameEligibleForSessionRag('a.jpg'), isFalse);
      expect(filenameEligibleForSessionRag('binary'), isFalse);
      expect(filenameEligibleForSessionRag(''), isFalse);
    });
  });
}
