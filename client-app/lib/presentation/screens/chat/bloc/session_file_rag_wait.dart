import 'package:gen/domain/entities/file_ingestion_status.dart';
import 'package:gen/domain/usecases/chat/get_file_ingestion_status_usecase.dart';

bool filenameEligibleForSessionRag(String name) {
  final lower = name.toLowerCase();
  const exts = [
    '.pdf',
    '.docx',
    '.txt',
    '.md',
    '.log',
    '.xlsx',
    '.csv',
    '.pptx',
    '.html',
    '.htm',
    '.xhtml',
  ];
  for (final e in exts) {
    if (lower.endsWith(e)) {
      return true;
    }
  }
  return false;
}

Future<bool> waitForSessionFileRagReady(
  int sessionId,
  int fileId,
  GetFileIngestionStatusUseCase getStatus, {
  void Function(FileIngestionStatus status)? onPoll,
}) async {
  const maxWait = Duration(seconds: 90);
  const step = Duration(milliseconds: 500);
  final sw = Stopwatch()..start();
  while (sw.elapsed < maxWait) {
    try {
      final s = await getStatus(sessionId: sessionId, fileId: fileId);
      onPoll?.call(s);
      if (s.status == 'ready') {
        return true;
      }

      if (s.status == 'failed' || s.status == 'unavailable') {
        return false;
      }
    } catch (_) {
      onPoll?.call(
        const FileIngestionStatus(
          status: 'error',
          lastError: 'Не удалось получить статус индексации',
        ),
      );
      return false;
    }
    await Future<void>.delayed(step);
  }

  onPoll?.call(const FileIngestionStatus(status: 'timeout'));
  return false;
}
