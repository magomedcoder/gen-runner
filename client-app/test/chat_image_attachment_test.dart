import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:gen/core/chat_image_attachment.dart';
import 'package:gen/domain/entities/message.dart';

Message _msg({
  MessageRole role = MessageRole.user,
  String? attachmentMime,
  String? attachmentFileName,
  List<int>? attachmentContent,
  int? attachmentFileId,
}) {
  return Message(
    id: 1,
    content: 'c',
    role: role,
    createdAt: DateTime.utc(2020),
    attachmentMime: attachmentMime,
    attachmentFileName: attachmentFileName,
    attachmentContent: attachmentContent != null
        ? Uint8List.fromList(attachmentContent)
        : null,
    attachmentFileId: attachmentFileId,
  );
}

void main() {
  group('guessImageMimeFromFilename', () {
    test('null and empty', () {
      expect(guessImageMimeFromFilename(null), isNull);
      expect(guessImageMimeFromFilename(''), isNull);
      expect(guessImageMimeFromFilename('   '), isNull);
    });

    test('extensions', () {
      expect(guessImageMimeFromFilename('X.PNG'), 'image/png');
      expect(guessImageMimeFromFilename('a.JPEG'), 'image/jpeg');
      expect(guessImageMimeFromFilename('b.jpg'), 'image/jpeg');
      expect(guessImageMimeFromFilename('c.webp'), 'image/webp');
      expect(guessImageMimeFromFilename('d.gif'), 'image/gif');
    });

    test('non-image', () {
      expect(guessImageMimeFromFilename('doc.pdf'), isNull);
      expect(guessImageMimeFromFilename('image'), isNull);
    });
  });

  group('messageEligibleForChatImageThumb', () {
    test('assistant never', () {
      expect(
        messageEligibleForChatImageThumb(
          _msg(role: MessageRole.assistant, attachmentMime: 'image/png'),
        ),
        isFalse,
      );
    });

    test('user with image mime', () {
      expect(
        messageEligibleForChatImageThumb(_msg(attachmentMime: 'Image/PNG')),
        isTrue,
      );
    });

    test('user with filename only', () {
      expect(
        messageEligibleForChatImageThumb(
          _msg(attachmentFileName: 'shot.jpeg'),
        ),
        isTrue,
      );
    });

    test('user without image hint', () {
      expect(messageEligibleForChatImageThumb(_msg()), isFalse);
    });
  });

  group('messageHasImageBytesOrFileRef', () {
    test('bytes', () {
      expect(
        messageHasImageBytesOrFileRef(_msg(attachmentContent: [1])),
        isTrue,
      );
    });

    test('file id', () {
      expect(messageHasImageBytesOrFileRef(_msg(attachmentFileId: 1)), isTrue);
    });

    test('neither', () {
      expect(messageHasImageBytesOrFileRef(_msg()), isFalse);
    });
  });
}
