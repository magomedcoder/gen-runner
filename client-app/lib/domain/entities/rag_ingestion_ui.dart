import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/file_ingestion_status.dart';

enum RagIngestionPhase {
  uploadingFile,
  queued,
  indexing,
  ready,
  willSendWithoutRag,
}

class RagIngestionUi extends Equatable {
  const RagIngestionUi({
    required this.fileName,
    required this.phase,
    this.detail = '',
    this.chunkCount = 0,
  });

  final String fileName;
  final RagIngestionPhase phase;
  final String detail;
  final int chunkCount;

  factory RagIngestionUi.uploading(String fileName) {
    return RagIngestionUi(
      fileName: fileName,
      phase: RagIngestionPhase.uploadingFile,
    );
  }

  factory RagIngestionUi.fromPoll(String fileName, FileIngestionStatus s) {
    switch (s.status) {
      case 'pending':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.queued,
          chunkCount: s.chunkCount,
        );
      case 'indexing':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.indexing,
          chunkCount: s.chunkCount,
        );
      case 'ready':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.ready,
          chunkCount: s.chunkCount,
        );
      case 'failed':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.willSendWithoutRag,
          detail: s.lastError.trim().isNotEmpty
              ? s.lastError.trim()
              : 'Индексация не удалась',
        );
      case 'unavailable':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.willSendWithoutRag,
          detail: 'Поиск по документу недоступен',
        );
      case 'timeout':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.willSendWithoutRag,
          detail: 'Индексация не завершилась вовремя - сообщение уйдёт без поиска по документу',
        );
      case 'error':
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.willSendWithoutRag,
          detail: s.lastError.trim().isNotEmpty
              ? s.lastError.trim()
              : 'Не удалось получить статус индексации',
        );
      default:
        return RagIngestionUi(
          fileName: fileName,
          phase: RagIngestionPhase.indexing,
          detail: s.status,
          chunkCount: s.chunkCount,
        );
    }
  }

  RagIngestionUi copyWith({
    String? fileName,
    RagIngestionPhase? phase,
    String? detail,
    int? chunkCount,
  }) {
    return RagIngestionUi(
      fileName: fileName ?? this.fileName,
      phase: phase ?? this.phase,
      detail: detail ?? this.detail,
      chunkCount: chunkCount ?? this.chunkCount,
    );
  }

  @override
  List<Object?> get props => [fileName, phase, detail, chunkCount];
}
