import 'package:gen/domain/entities/file_ingestion_status.dart';
import 'package:gen/domain/repositories/chat_repository.dart';

class GetFileIngestionStatusUseCase {
  final ChatRepository _repository;

  GetFileIngestionStatusUseCase(this._repository);

  Future<FileIngestionStatus> call({
    required int sessionId,
    required int fileId,
  }) {
    return _repository.getFileIngestionStatus(
      sessionId: sessionId,
      fileId: fileId,
    );
  }
}
