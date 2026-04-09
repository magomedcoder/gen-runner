import 'package:equatable/equatable.dart';

class FileIngestionStatus extends Equatable {
  final String status;
  final String lastError;
  final int chunkCount;
  final String sourceContentSha256;
  final String pipelineVersion;
  final String embeddingModel;

  const FileIngestionStatus({
    required this.status,
    this.lastError = '',
    this.chunkCount = 0,
    this.sourceContentSha256 = '',
    this.pipelineVersion = '',
    this.embeddingModel = '',
  });

  @override
  List<Object?> get props => [
    status,
    lastError,
    chunkCount,
    sourceContentSha256,
    pipelineVersion,
    embeddingModel,
  ];
}
