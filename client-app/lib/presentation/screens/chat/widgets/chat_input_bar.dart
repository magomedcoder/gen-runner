import 'dart:convert';
import 'dart:math' as math;

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/attachment_settings.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_bloc.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_event.dart';
import 'package:gen/presentation/screens/chat/bloc/chat_state.dart';

class ChatInputBar extends StatefulWidget {
  const ChatInputBar({super.key, required this.isEnabled});

  final bool isEnabled;

  @override
  State<ChatInputBar> createState() => ChatInputBarState();
}

class ChatInputBarState extends State<ChatInputBar> {
  static const double _inputCardMinHeight = 92.0;
  static const double _inputCardGrowthStep = 50.0;
  static const double _inputCardMaxWindowFactor = 0.5;

  final _textController = TextEditingController();
  final _focusNode = FocusNode();
  bool _isComposing = false;
  PlatformFile? _selectedFile;

  @override
  void initState() {
    super.initState();
    _textController.addListener(_onTextChanged);
  }

  void _onTextChanged() {
    setState(() {
      _isComposing = _textController.text.trim().isNotEmpty;
    });
  }

  void _insertNewlineAtCursor() {
    if (!widget.isEnabled) {
      return;
    }
    final v = _textController.value;
    final text = v.text;
    final sel = v.selection;
    if (!sel.isValid) {
      _textController.value = TextEditingValue(
        text: '$text\n',
        selection: TextSelection.collapsed(offset: text.length + 1),
      );
      return;
    }
    final start = sel.start;
    final end = sel.end;
    final newText = text.replaceRange(start, end, '\n');
    _textController.value = TextEditingValue(
      text: newText,
      selection: TextSelection.collapsed(offset: start + 1),
    );
  }

  Future<void> _sendMessage() async {
    final text = _textController.text.trim();
    final hasFile = _selectedFile != null;

    if (text.isEmpty && !hasFile) {
      return;
    }

    if (hasFile) {
      final file = _selectedFile!;
      final bytes = file.bytes;
      if (bytes == null) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('Не удалось прочитать файл. Попробуйте снова.')),
          );
        }
        return;
      }

      if (bytes.length > AttachmentSettings.maxFileSizeBytes) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Файл слишком большой (рекомендуется до ${AttachmentSettings.maxFileSizeLabel})'),
            ),
          );
        }

        return;
      }

      if (!AttachmentSettings.isBinaryDocument(file.name)) {
        try {
          utf8.decode(bytes);
        } on FormatException {
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(content: Text('Неподдерживаемый формат')),
            );
          }
          return;
        }
      }
    }

    context.read<ChatBloc>().add(
      ChatSendMessage(
        text,
        attachmentFileName: hasFile ? _selectedFile!.name : null,
        attachmentContent: hasFile ? _selectedFile!.bytes : null,
      ),
    );
    _textController.clear();
    _focusNode.unfocus();
    setState(() => _selectedFile = null);
  }

  Future<void> _pickFile() async {
    if (!widget.isEnabled) {
      return;
    }

    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: AttachmentSettings.textFileExtensions,
      allowMultiple: false,
      withData: true,
    );

    if (result == null) {
      return;
    }

    final file = result.files.single;
    if (!AttachmentSettings.isSupportedExtension(file.name)) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Неподдерживаемый формат. Доступно: ${AttachmentSettings.textFormatLabels.join(', ')}, ${AttachmentSettings.documentFormatLabels.join(', ')}',
            ),
          ),
        );
      }
      return;
    }

    if (file.bytes == null) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Не удалось загрузить содержимое файла'),
          ),
        );
      }
      return;
    }

    if (file.bytes!.length > AttachmentSettings.maxFileSizeBytes) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Файл слишком большой (рекомендуется до ${AttachmentSettings.maxFileSizeLabel})',
            ),
          ),
        );
      }
      return;
    }
    setState(() => _selectedFile = file);
  }

  void _clearFile() {
    setState(() => _selectedFile = null);
  }

  void resetComposer() {
    if (!mounted) {
      return;
    }
    _textController.clear();
    setState(() => _selectedFile = null);
  }

  void setDroppedFile(PlatformFile file) {
    if (!widget.isEnabled) {
      return;
    }

    if (file.bytes == null || file.bytes!.isEmpty) {
      return;
    }

    if (!AttachmentSettings.isSupportedExtension(file.name)) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Неподдерживаемый формат. Доступно: ${AttachmentSettings.textFormatLabels.join(', ')}, ${AttachmentSettings.documentFormatLabels.join(', ')}',
            ),
          ),
        );
      }
      return;
    }

    if (file.bytes!.length > AttachmentSettings.maxFileSizeBytes) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Файл слишком большой (рекомендуется до ${AttachmentSettings.maxFileSizeLabel})',
            ),
          ),
        );
      }
      return;
    }
    if (!AttachmentSettings.isBinaryDocument(file.name)) {
      try {
        utf8.decode(file.bytes!);
      } on FormatException {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('Неподдерживаемый формат')),
          );
        }
        return;
      }
    }
    setState(() => _selectedFile = file);
  }

  void _stopGeneration() {
    context.read<ChatBloc>().add(const ChatStopGeneration());
  }

  @override
  void dispose() {
    _textController.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  double _cardMaxHeight(BuildContext context) {
    final h = MediaQuery.sizeOf(context).height;
    return math.max(_inputCardMinHeight, h * _inputCardMaxWindowFactor);
  }

  int _estimatedLineCount({
    required BuildContext context,
    required TextStyle textStyle,
    required double availableWidth,
  }) {
    final text = _textController.text;
    if (text.isEmpty) {
      return 1;
    }

    final painter = TextPainter(
      text: TextSpan(text: text, style: textStyle),
      textDirection: Directionality.of(context),
      maxLines: null,
    )..layout(maxWidth: availableWidth);

    return math.max(1, painter.computeLineMetrics().length);
  }

  double _cardHeightForText(
    BuildContext context, {
    required TextStyle textStyle,
    required double horizontalPadding,
  }) {
    final windowWidth = MediaQuery.sizeOf(context).width;
    final availableTextWidth = math.max(120.0, windowWidth - (horizontalPadding * 2) - 24.0);
    final lines = _estimatedLineCount(
      context: context,
      textStyle: textStyle,
      availableWidth: availableTextWidth,
    );
    final attachmentExtra = _selectedFile == null ? 0.0 : 36.0;
    final targetHeight = _inputCardMinHeight + attachmentExtra + ((lines - 1) * _inputCardGrowthStep);
    final maxHeight = _cardMaxHeight(context);

    return targetHeight.clamp(_inputCardMinHeight, maxHeight);
  }

  Widget _buildAttachmentChip(ThemeData theme) {
    if (_selectedFile == null) {
      return const SizedBox.shrink();
    }

    return Padding(
      padding: const EdgeInsets.fromLTRB(6, 6, 6, 0),
      child: Material(
        color: theme.colorScheme.primaryContainer.withValues(alpha: 0.45),
        borderRadius: BorderRadius.circular(10),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
          child: Row(
            children: [
              Icon(
                Icons.insert_drive_file_rounded,
                size: 18,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  _selectedFile!.name,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: theme.colorScheme.onPrimaryContainer,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              IconButton(
                visualDensity: VisualDensity.compact,
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
                icon: Icon(
                  Icons.close_rounded,
                  size: 18,
                ),
                onPressed: _clearFile,
                tooltip: 'Убрать файл',
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildBottomActionsBar(ChatState state, ThemeData theme) {
    final canSend = (_isComposing || _selectedFile != null) && widget.isEnabled;

    return Material(
      color: Colors.transparent,
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.only(right: 12),
        decoration: BoxDecoration(
          border: Border(
            top: BorderSide(color: theme.dividerColor.withValues(alpha: 0.12)),
          ),
        ),
        child: Row(
          children: [
            IconButton(
              tooltip: 'Прикрепить файл',
              onPressed: widget.isEnabled ? _pickFile : null,
              icon: Icon(
                Icons.attach_file_rounded,
                color: widget.isEnabled
                  ? theme.colorScheme.onSurfaceVariant
                  : theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.4),
              ),
            ),
            if (state.retryText != null && !state.isStreaming) ...[
              TextButton.icon(
                onPressed: widget.isEnabled
                  ? () => context.read<ChatBloc>().add(const ChatRetryLastMessage())
                  : null,
                icon: const Icon(Icons.refresh_rounded, size: 18),
                label: const Text('Повторить'),
              ),
              const SizedBox(width: 6),
            ],
            Expanded(
              child: LayoutBuilder(
                builder: (context, constraints) {
                  return Align(
                    alignment: Alignment.centerRight,
                    child: ConstrainedBox(
                      constraints: BoxConstraints(
                        maxWidth: constraints.maxWidth,
                      ),
                      child: state.isStreaming
                        ? FilledButton.tonal(
                          onPressed: _stopGeneration,
                          style: FilledButton.styleFrom(
                            visualDensity: VisualDensity.compact,
                            padding: const EdgeInsets.symmetric(
                              horizontal: 10,
                              vertical: 4,
                            ),
                            backgroundColor: theme.colorScheme.errorContainer,
                            foregroundColor: theme.colorScheme.onErrorContainer,
                          ),
                          child: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const Icon(Icons.stop_rounded, size: 20),
                              const SizedBox(width: 8),
                              Flexible(
                                child: Text(
                                  'Стоп',
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: TextStyle(
                                    fontWeight: FontWeight.w500,
                                    color: theme
                                        .colorScheme.onErrorContainer,
                                  ),
                                ),
                              ),
                            ],
                          ),
                        )
                        : FilledButton(
                          onPressed: canSend ? _sendMessage : null,
                          style: FilledButton.styleFrom(
                            visualDensity: VisualDensity.compact,
                            padding: const EdgeInsets.symmetric(
                              horizontal: 10,
                              vertical: 4,
                            ),
                          ),
                          child: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const Icon(Icons.send_rounded, size: 20),
                              const SizedBox(width: 8),
                              Flexible(
                                child: Text(
                                  'Отправить',
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  textAlign: TextAlign.center,
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w500,
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                    ),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final horizontal = Breakpoints.isMobile(context) ? 12.0 : 20.0;
    final theme = Theme.of(context);
    final isDesktop = !Breakpoints.isMobile(context);
    final inputTextStyle = TextStyle(
      fontSize: 15,
      height: 1.45,
      letterSpacing: 0.15,
      color: widget.isEnabled
        ? theme.colorScheme.onSurface
        : theme.colorScheme.onSurfaceVariant,
    );

    return BlocBuilder<ChatBloc, ChatState>(
      builder: (context, state) {
        final cardHeight = _cardHeightForText(
          context,
          textStyle: inputTextStyle,
          horizontalPadding: horizontal,
        );
        return AnimatedContainer(
          duration: const Duration(milliseconds: 140),
          curve: Curves.easeOutCubic,
          height: cardHeight,
          constraints: BoxConstraints(
            maxHeight: _cardMaxHeight(context),
            minHeight: _inputCardMinHeight,
          ),
          child: DecoratedBox(
            decoration: BoxDecoration(
              color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.35),
            ),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.max,
                children: [
                  _buildAttachmentChip(theme),
                  Expanded(
                    child: CallbackShortcuts(
                      bindings: {
                        const SingleActivator(
                          LogicalKeyboardKey.enter,
                          shift: true,
                        ): _insertNewlineAtCursor,
                        const SingleActivator(
                          LogicalKeyboardKey.numpadEnter,
                          shift: true,
                        ): _insertNewlineAtCursor,
                        if (isDesktop) ...{
                          const SingleActivator(
                            LogicalKeyboardKey.enter,
                            control: true,
                          ): _insertNewlineAtCursor,
                          const SingleActivator(
                            LogicalKeyboardKey.enter,
                            meta: true,
                          ): _insertNewlineAtCursor,
                          const SingleActivator(
                            LogicalKeyboardKey.numpadEnter,
                            control: true,
                          ): _insertNewlineAtCursor,
                          const SingleActivator(
                            LogicalKeyboardKey.numpadEnter,
                            meta: true,
                          ): _insertNewlineAtCursor,
                        },
                        const SingleActivator(LogicalKeyboardKey.enter): () {
                          if (widget.isEnabled) {
                            _sendMessage();
                          }
                        },
                        const SingleActivator(
                          LogicalKeyboardKey.numpadEnter,
                        ): () {
                          if (widget.isEnabled) {
                            _sendMessage();
                          }
                        },
                      },
                      child: TextField(
                        controller: _textController,
                        focusNode: _focusNode,
                        enabled: widget.isEnabled,
                        expands: true,
                        maxLines: null,
                        minLines: null,
                        textAlignVertical: TextAlignVertical.top,
                        style: inputTextStyle,
                        decoration: InputDecoration(
                          hintText: widget.isEnabled
                            ? (isDesktop ? 'Сообщение...  Ctrl+Enter - новая строка' : 'Сообщение…')
                            : 'Обрабатываю...',
                          hintStyle: TextStyle(
                            fontSize: 14,
                            height: 1.45,
                            color: theme.colorScheme.onSurface.withValues(alpha: 0.45),
                          ),
                          border: InputBorder.none,
                          focusedBorder: InputBorder.none,
                          isDense: true,
                          contentPadding: const EdgeInsets.fromLTRB(10, 12, 10, 4),
                        ),
                        textInputAction: TextInputAction.newline,
                        keyboardType: TextInputType.multiline,
                        scrollPhysics: const BouncingScrollPhysics(
                          parent: AlwaysScrollableScrollPhysics(),
                        ),
                        onTapOutside: (_) => _focusNode.unfocus(),
                      ),
                    ),
                  ),
                  _buildBottomActionsBar(state, theme),
                ],
              ),
            ),
          ),
        );
      },
    );
  }
}

