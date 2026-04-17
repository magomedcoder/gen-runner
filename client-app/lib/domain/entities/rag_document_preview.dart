import 'dart:convert';

import 'package:equatable/equatable.dart';

class RagSourcesPayloadSnapshot extends Equatable {
  const RagSourcesPayloadSnapshot({
    required this.mode,
    required this.fileId,
    required this.topK,
    required this.neighborWindow,
    required this.deepRagMapCalls,
    required this.droppedByBudget,
    required this.fullDocumentExcerpt,
    required this.chunks,
  });

  final String mode;
  final int fileId;
  final int topK;
  final int neighborWindow;
  final int deepRagMapCalls;
  final int droppedByBudget;
  final String fullDocumentExcerpt;
  final List<RagChunkRef> chunks;

  @override
  List<Object?> get props => [
    mode,
    fileId,
    topK,
    neighborWindow,
    deepRagMapCalls,
    droppedByBudget,
    fullDocumentExcerpt,
    chunks,
  ];
}

class RagChunkRef extends Equatable {
  const RagChunkRef({
    required this.chunkIndex,
    required this.score,
    required this.isNeighbor,
    this.headingPath = '',
    this.pdfPageStart = 0,
    this.pdfPageEnd = 0,
    this.excerpt = '',
  });

  final int chunkIndex;
  final double score;
  final bool isNeighbor;
  final String headingPath;
  final int pdfPageStart;
  final int pdfPageEnd;
  final String excerpt;

  @override
  List<Object?> get props => [
    chunkIndex,
    score,
    isNeighbor,
    headingPath,
    pdfPageStart,
    pdfPageEnd,
    excerpt,
  ];
}

class RagDocumentPreview extends Equatable {
  const RagDocumentPreview({
    required this.mode,
    required this.summary,
    required this.fileId,
    required this.fullDocumentExcerpt,
    required this.topK,
    required this.neighborWindow,
    required this.deepRagMapCalls,
    required this.chunks,
  });

  final String mode;
  final String summary;
  final int fileId;
  final String fullDocumentExcerpt;
  final int topK;
  final int neighborWindow;
  final int deepRagMapCalls;
  final List<RagChunkRef> chunks;

  bool get isFullDocument => mode == 'full_document';

  static int _int(dynamic v) {
    if (v is int) {
      return v;
    }
    if (v is num) {
      return v.toInt();
    }
    return 0;
  }

  static double _double(dynamic v) {
    if (v is double) {
      return v;
    }
    if (v is num) {
      return v.toDouble();
    }
    return 0;
  }

  static bool _bool(dynamic v) {
    if (v is bool) {
      return v;
    }
    return false;
  }

  factory RagDocumentPreview.fromPayloadSnapshot({
    required String summary,
    String? modeFromStream,
    required RagSourcesPayloadSnapshot payload,
  }) {
    final m = payload.mode.trim().isNotEmpty
        ? payload.mode.trim()
        : (modeFromStream ?? '').trim();
    return RagDocumentPreview(
      mode: m,
      summary: summary.trim(),
      fileId: payload.fileId,
      fullDocumentExcerpt: payload.fullDocumentExcerpt,
      topK: payload.topK,
      neighborWindow: payload.neighborWindow,
      deepRagMapCalls: payload.deepRagMapCalls,
      chunks: payload.chunks,
    );
  }

  static RagDocumentPreview? tryParse({
    required String summary,
    String? sourcesJson,
    String? modeFromStream,
    RagSourcesPayloadSnapshot? ragSources,
  }) {
    if (ragSources != null) {
      return RagDocumentPreview.fromPayloadSnapshot(
        summary: summary,
        modeFromStream: modeFromStream,
        payload: ragSources,
      );
    }
    final trimmedSummary = summary.trim();
    final raw = sourcesJson?.trim();
    if (raw == null || raw.isEmpty) {
      final m = modeFromStream?.trim() ?? '';
      if (m.isEmpty && trimmedSummary.isEmpty) {
        return null;
      }
      return RagDocumentPreview(
        mode: m,
        summary: trimmedSummary,
        fileId: 0,
        fullDocumentExcerpt: '',
        topK: 0,
        neighborWindow: 0,
        deepRagMapCalls: 0,
        chunks: const [],
      );
    }
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map<String, dynamic>) {
        return _fallback(
          summary: trimmedSummary,
          modeFromStream: modeFromStream,
        );
      }
      final o = decoded;
      final mode =
          (o['mode'] as String?)?.trim() ?? modeFromStream?.trim() ?? '';
      final topK = _int(o['top_k']);
      final neighborWindow = _int(o['neighbor_window']);
      final deep = _int(o['deep_rag_map_calls']);
      final fileId = _int(o['file_id']);
      final fde = o['full_document_excerpt'];
      final fullDocumentExcerpt = fde is String ? fde.trim() : '';
      final rawChunks = o['chunks'];
      final chunks = <RagChunkRef>[];
      if (rawChunks is List) {
        for (final e in rawChunks) {
          if (e is! Map<String, dynamic>) {
            continue;
          }
          final hp = e['heading_path'];
          final ex = e['excerpt'];
          chunks.add(
            RagChunkRef(
              chunkIndex: _int(e['chunk_index']),
              score: _double(e['score']),
              isNeighbor: _bool(e['is_neighbor']),
              headingPath: hp is String ? hp.trim() : '',
              pdfPageStart: _int(e['pdf_page_start']),
              pdfPageEnd: _int(e['pdf_page_end']),
              excerpt: ex is String ? ex.trim() : '',
            ),
          );
        }
      }
      return RagDocumentPreview(
        mode: mode,
        summary: trimmedSummary,
        fileId: fileId,
        fullDocumentExcerpt: fullDocumentExcerpt,
        topK: topK,
        neighborWindow: neighborWindow,
        deepRagMapCalls: deep,
        chunks: chunks,
      );
    } catch (_) {
      return _fallback(summary: trimmedSummary, modeFromStream: modeFromStream);
    }
  }

  static RagDocumentPreview _fallback({
    required String summary,
    String? modeFromStream,
  }) {
    return RagDocumentPreview(
      mode: modeFromStream?.trim() ?? '',
      summary: summary.trim(),
      fileId: 0,
      fullDocumentExcerpt: '',
      topK: 0,
      neighborWindow: 0,
      deepRagMapCalls: 0,
      chunks: const [],
    );
  }

  @override
  List<Object?> get props => [
    mode,
    summary,
    fileId,
    fullDocumentExcerpt,
    topK,
    neighborWindow,
    deepRagMapCalls,
    chunks,
  ];
}
