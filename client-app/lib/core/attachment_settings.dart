abstract final class AttachmentSettings {
  AttachmentSettings._();

  static const int maxFileSizeBytes = 2 * 1024 * 1024;

  static int get maxFileSizeKb => maxFileSizeBytes ~/ 1024;
  static String get maxFileSizeLabel {
    final mb = maxFileSizeBytes / (1024 * 1024);
    if (mb >= 1) {
      return '${mb.toStringAsFixed(mb % 1 == 0 ? 0 : 1)} МБ';
    }

    return '$maxFileSizeKb КБ';
  }

  static const List<String> textFileExtensions = [
    'txt',
    'md',
    'log',
    'json',
    'pdf',
    'docx',
    'xlsx',
    'csv',
    'pptx',
    'html',
    'htm',
    'xhtml',
  ];

  static const List<String> imageAttachmentExtensions = [
    'png',
    'jpg',
    'jpeg',
    'webp',
    'gif',
  ];

  static const List<String> documentBinaryExtensions = [
    'pdf',
    'docx',
    'xlsx',
    'csv',
    'pptx',
  ];

  static bool isBinaryDocument(String filename) {
    final parts = filename.split('.');
    if (parts.length < 2) {
      return false;
    }
    final ext = parts.last.toLowerCase();

    return documentBinaryExtensions.contains(ext);
  }

  static bool isSupportedExtension(String filename) {
    final parts = filename.split('.');
    if (parts.length < 2) {
      return false;
    }

    final ext = parts.last.toLowerCase();

    return textFileExtensions.contains(ext) ||
        imageAttachmentExtensions.contains(ext);
  }

  static List<String> get allChatAttachmentExtensions => [
    ...textFileExtensions,
    ...imageAttachmentExtensions,
  ];

  static const List<String> textFormatLabels = [
    'TXT',
    'MD',
    'LOG',
    'JSON',
    'HTML',
  ];

  static const List<String> documentFormatLabels = [
    'PDF',
    'DOCX',
    'XLSX',
    'CSV',
    'PPTX',
  ];

  static const List<String> imageFormatLabels = ['PNG', 'JPEG', 'WebP', 'GIF'];
}
