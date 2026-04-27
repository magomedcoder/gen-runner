import 'package:flutter_test/flutter_test.dart';
import 'package:gen/core/tool_chain_partition.dart';
import 'package:gen/domain/entities/message.dart';

Message _a(int id, String content, String toolCallsJson) => Message(
  id: id,
  content: content,
  role: MessageRole.assistant,
  createdAt: DateTime(2020),
  toolCallsJson: toolCallsJson,
);

Message _t(int id, String content) => Message(
  id: id,
  content: content,
  role: MessageRole.tool,
  createdAt: DateTime(2020),
);

Message _u(int id, String content) => Message(
  id: id,
  content: content,
  role: MessageRole.user,
  createdAt: DateTime(2020),
);

void main() {
  const tc =
      '[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]';

  test('two sequential rounds merge into one chain', () {
    final msgs = [
      _a(1, 'r1', tc),
      _t(2, 't1'),
      _a(3, 'r2', tc),
      _t(4, 't2'),
      _a(5, 'final', ''),
    ];
    final p = partitionMessagesForToolChainUi(msgs);
    expect(p.length, 1);
    expect(p.first, isA<PartitionToolChain>());
    final ch = (p.first as PartitionToolChain).chain;
    expect(ch.segments.length, 2);
    expect(ch.finalAssistantIndex, 4);
  });

  test('user message breaks chain', () {
    final msgs = [
      _a(1, 'r1', tc),
      _t(2, 't1'),
      _u(3, 'hi'),
      _a(4, 'final', ''),
    ];
    final p = partitionMessagesForToolChainUi(msgs);
    expect(p.length, 3);
    expect(p[0], isA<PartitionToolChain>());
    expect(p[1], isA<PartitionSingle>());
    expect((p[1] as PartitionSingle).index, 2);
    expect(p[2], isA<PartitionSingle>());
    expect((p[2] as PartitionSingle).index, 3);
  });
}
