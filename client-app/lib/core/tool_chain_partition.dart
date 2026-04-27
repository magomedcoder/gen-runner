import 'package:gen/domain/entities/message.dart';

class ToolSegmentIndices {
  const ToolSegmentIndices({
    required this.leadIndex,
    required this.toolStart,
    required this.toolEnd,
  });

  final int leadIndex;
  final int toolStart;
  final int toolEnd;
}

class ToolChainIndices {
  const ToolChainIndices({
    required this.segments,
    this.finalAssistantIndex,
  });

  final List<ToolSegmentIndices> segments;
  final int? finalAssistantIndex;
}

sealed class MessagePartition {}

class PartitionSingle extends MessagePartition {
  PartitionSingle(this.index);
  final int index;
}

class PartitionToolChain extends MessagePartition {
  PartitionToolChain(this.chain);
  final ToolChainIndices chain;
}

bool _assistantHasToolCalls(Message m) {
  if (m.role != MessageRole.assistant) {
    return false;
  }

  return m.toolCallsJson != null && m.toolCallsJson!.trim().isNotEmpty;
}

class _ConsumeResult {
  const _ConsumeResult(this.chain, this.nextIndex);
  final ToolChainIndices chain;
  final int nextIndex;
}

_ConsumeResult? _consumeToolChain(List<Message> msgs, int start) {
  final n = msgs.length;
  var cur = start;
  final segs = <ToolSegmentIndices>[];

  while (true) {
    if (cur >= n || !_assistantHasToolCalls(msgs[cur])) {
      if (segs.isEmpty) {
        return null;
      }

      return _ConsumeResult(ToolChainIndices(segments: segs), cur);
    }

    var te = cur + 1;
    while (te < n && msgs[te].role == MessageRole.tool) {
      te++;
    }

    if (te == cur + 1) {
      if (segs.isEmpty) {
        return null;
      }
      return _ConsumeResult(ToolChainIndices(segments: segs), cur);
    }

    segs.add(ToolSegmentIndices(leadIndex: cur, toolStart: cur + 1, toolEnd: te - 1));

    if (te >= n) {
      return _ConsumeResult(ToolChainIndices(segments: segs), n);
    }

    final nxt = msgs[te];
    if (nxt.role != MessageRole.assistant) {
      return _ConsumeResult(ToolChainIndices(segments: segs), te);
    }

    if (_assistantHasToolCalls(nxt)) {
      cur = te;
      continue;
    }

    return _ConsumeResult(
      ToolChainIndices(segments: segs, finalAssistantIndex: te),
      te + 1,
    );
  }
}

List<MessagePartition> partitionMessagesForToolChainUi(List<Message> msgs) {
  final n = msgs.length;
  final out = <MessagePartition>[];
  var i = 0;
  while (i < n) {
    final m = msgs[i];
    if (!_assistantHasToolCalls(m)) {
      out.add(PartitionSingle(i));
      i++;
      continue;
    }

    final consumed = _consumeToolChain(msgs, i);
    if (consumed == null) {
      out.add(PartitionSingle(i));
      i++;
      continue;
    }

    out.add(PartitionToolChain(consumed.chain));
    i = consumed.nextIndex;
  }

  return out;
}
