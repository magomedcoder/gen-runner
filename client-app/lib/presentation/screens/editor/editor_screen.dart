import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:gen/core/layout/responsive.dart';
import 'package:gen/generated/grpc_pb/editor.pb.dart' as grpc;
import 'package:gen/presentation/screens/editor/bloc/editor_bloc.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_event.dart';
import 'package:gen/presentation/screens/editor/bloc/editor_state.dart';

class EditorScreen extends StatefulWidget {
  const EditorScreen({super.key});

  @override
  State<EditorScreen> createState() => _EditorScreenState();
}

class _EditorScreenState extends State<EditorScreen> {
  final _controller = TextEditingController();
  Timer? _debounce;
  int _lastDocVersion = 0;
  bool _applyingFromBloc = false;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onLocalTextChanged);
  }

  void _onLocalTextChanged() {
    if (_applyingFromBloc) return;
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 350), () {
      if (!mounted) return;
      context.read<EditorBloc>().add(EditorDocumentChanged(_controller.text));
    });
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _controller.removeListener(_onLocalTextChanged);
    _controller.dispose();
    super.dispose();
  }

  void _flushDocumentToBloc() {
    _debounce?.cancel();
    context.read<EditorBloc>().add(EditorDocumentChanged(_controller.text));
  }

  String _labelForType(grpc.TransformType t) {
    if (t == grpc.TransformType.TRANSFORM_TYPE_FIX) return 'Исправить';
    if (t == grpc.TransformType.TRANSFORM_TYPE_IMPROVE) return 'Улучшить';
    if (t == grpc.TransformType.TRANSFORM_TYPE_BEAUTIFY) return 'Красиво';
    if (t == grpc.TransformType.TRANSFORM_TYPE_PARAPHRASE) {
      return 'Другими словами';
    }
    if (t == grpc.TransformType.TRANSFORM_TYPE_SHORTEN) return 'Кратко';
    if (t == grpc.TransformType.TRANSFORM_TYPE_SIMPLIFY) return 'Проще';
    if (t == grpc.TransformType.TRANSFORM_TYPE_MAKE_COMPLEX) return 'Сложнее';
    if (t == grpc.TransformType.TRANSFORM_TYPE_MORE_FORMAL) {
      return 'Более формально';
    }
    if (t == grpc.TransformType.TRANSFORM_TYPE_MORE_CASUAL) {
      return 'Разговорный стиль';
    }
    return 'Выберите режим';
  }

  static const List<grpc.TransformType> _types = [
    grpc.TransformType.TRANSFORM_TYPE_FIX,
    grpc.TransformType.TRANSFORM_TYPE_IMPROVE,
    grpc.TransformType.TRANSFORM_TYPE_BEAUTIFY,
    grpc.TransformType.TRANSFORM_TYPE_PARAPHRASE,
    grpc.TransformType.TRANSFORM_TYPE_SHORTEN,
    grpc.TransformType.TRANSFORM_TYPE_SIMPLIFY,
    grpc.TransformType.TRANSFORM_TYPE_MAKE_COMPLEX,
    grpc.TransformType.TRANSFORM_TYPE_MORE_FORMAL,
    grpc.TransformType.TRANSFORM_TYPE_MORE_CASUAL,
  ];

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return BlocConsumer<EditorBloc, EditorState>(
      listenWhen: (p, c) =>
          p.documentVersion != c.documentVersion || p.error != c.error,
      listener: (context, state) {
        if (state.documentVersion != _lastDocVersion) {
          _lastDocVersion = state.documentVersion;
          final t = state.documentText;
          if (_controller.text != t) {
            _applyingFromBloc = true;
            _controller.value = TextEditingValue(
              text: t,
              selection: TextSelection.collapsed(offset: t.length),
            );
            _applyingFromBloc = false;
          }
        }

        if (state.error != null && state.error!.isNotEmpty) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(state.error!),
              backgroundColor: colorScheme.error,
            ),
          );
          context.read<EditorBloc>().add(const EditorClearError());
        }
      },
      builder: (context, state) {
        final isMobile = Breakpoints.isMobile(context);

        return Scaffold(
          body: SafeArea(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _buildTopBar(
                  context,
                  state,
                  theme,
                  colorScheme,
                  isMobile,
                ),
                Expanded(
                  child: _buildEditorPane(
                    context,
                    state,
                    theme,
                    colorScheme,
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  BoxDecoration _editorControlDecoration(
    ColorScheme colorScheme, {
    required bool embedded,
  }) {
    return BoxDecoration(
      color: embedded
          ? colorScheme.surface.withValues(alpha: 0.5)
          : colorScheme.surfaceContainerHighest,
      borderRadius: BorderRadius.circular(10),
      border: Border.all(
        color: colorScheme.outline.withValues(alpha: embedded ? 0.12 : 0.22),
      ),
    );
  }

  Widget _buildTopBar(
    BuildContext context,
    EditorState state,
    ThemeData theme,
    ColorScheme colorScheme,
    bool isMobile,
  ) {
    final onHeader = colorScheme.onSurface;
    final subtle = colorScheme.onSurfaceVariant;

    Widget titleBlock() {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          DecoratedBox(
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [
                  colorScheme.primary.withValues(alpha: 0.22),
                  colorScheme.tertiary.withValues(alpha: 0.14),
                ],
              ),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(
                color: colorScheme.primary.withValues(alpha: 0.28),
              ),
            ),
            child: Padding(
              padding: const EdgeInsets.all(10),
              child: Icon(
                Icons.auto_awesome_rounded,
                size: 22,
                color: colorScheme.primary,
              ),
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  'Редактор',
                  style: theme.textTheme.titleLarge?.copyWith(
                    fontWeight: FontWeight.w700,
                    letterSpacing: -0.3,
                    color: onHeader,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  'Исправление, стиль и перефразирование текста',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: subtle,
                    height: 1.25,
                  ),
                ),
              ],
            ),
          ),
          if (state.isLoading)
            Padding(
              padding: const EdgeInsets.only(left: 8),
              child: SizedBox(
                width: 22,
                height: 22,
                child: CircularProgressIndicator(
                  strokeWidth: 2.5,
                  color: colorScheme.primary,
                ),
              ),
            ),
        ],
      );
    }

    Widget toolbarCard({required List<Widget> children}) {
      return DecoratedBox(
        decoration: BoxDecoration(
          color: colorScheme.surfaceContainerHighest.withValues(alpha: 0.42),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: colorScheme.outlineVariant.withValues(alpha: 0.55),
          ),
        ),
        child: Padding(
          padding: EdgeInsets.symmetric(
            horizontal: isMobile ? 10 : 14,
            vertical: isMobile ? 10 : 11,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: children,
          ),
        ),
      );
    }

    return Material(
      color: colorScheme.surfaceContainerLow,
      child: DecoratedBox(
        decoration: BoxDecoration(
          border: Border(
            bottom: BorderSide(
              color: colorScheme.outline.withValues(alpha: 0.1),
            ),
          ),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.06),
              blurRadius: 12,
              offset: const Offset(0, 4),
            ),
          ],
        ),
        child: Padding(
          padding: EdgeInsets.fromLTRB(
            isMobile ? 14 : 22,
            isMobile ? 12 : 16,
            isMobile ? 14 : 22,
            isMobile ? 12 : 14,
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: [
              titleBlock(),
              SizedBox(height: isMobile ? 12 : 14),
              if (isMobile)
                toolbarCard(
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: _buildModeSelector(
                            context,
                            state,
                            embedded: true,
                          ),
                        ),
                        const SizedBox(width: 8),
                        Expanded(
                          child: _buildModelSelector(
                            context,
                            state,
                            embedded: true,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 10),
                    LayoutBuilder(
                      builder: (context, constraints) {
                        return Wrap(
                          spacing: 8,
                          runSpacing: 8,
                          crossAxisAlignment: WrapCrossAlignment.center,
                          children: [
                            _buildHistoryButtons(context, state, colorScheme),
                            _buildMarkdownToggle(context, state, embedded: true),
                            SizedBox(
                              width: constraints.maxWidth,
                              child: _buildPrimaryActions(context, state, true),
                            ),
                          ],
                        );
                      },
                    ),
                  ],
                )
              else
                toolbarCard(
                  children: [
                    Row(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        _buildHistoryButtons(context, state, colorScheme),
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 10),
                          child: SizedBox(
                            height: 28,
                            child: VerticalDivider(
                              width: 1,
                              thickness: 1,
                              color: colorScheme.outline.withValues(alpha: 0.2),
                            ),
                          ),
                        ),
                        SizedBox(
                          width: 200,
                          child: _buildModeSelector(
                            context,
                            state,
                            embedded: true,
                          ),
                        ),
                        const SizedBox(width: 10),
                        SizedBox(
                          width: 220,
                          child: _buildModelSelector(
                            context,
                            state,
                            embedded: true,
                          ),
                        ),
                        const Spacer(),
                        _buildMarkdownToggle(context, state, embedded: true),
                        const SizedBox(width: 10),
                        _buildPrimaryActions(context, state, false),
                      ],
                    ),
                  ],
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildHistoryButtons(
    BuildContext context,
    EditorState state,
    ColorScheme colorScheme,
  ) {
    final btnStyle = IconButton.styleFrom(
      visualDensity: VisualDensity.compact,
      minimumSize: const Size(40, 40),
      padding: const EdgeInsets.all(8),
      backgroundColor: colorScheme.surface.withValues(alpha: 0.55),
      foregroundColor: colorScheme.onSurfaceVariant,
      disabledForegroundColor: colorScheme.onSurface.withValues(alpha: 0.28),
      disabledBackgroundColor: colorScheme.surface.withValues(alpha: 0.2),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(10),
        side: BorderSide(
          color: colorScheme.outline.withValues(alpha: 0.12),
        ),
      ),
    );

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        IconButton(
          style: btnStyle,
          tooltip: 'Назад в истории',
          onPressed: state.isLoading || !state.canUndo
              ? null
              : () {
                  _flushDocumentToBloc();
                  context.read<EditorBloc>().add(const EditorUndo());
                },
          icon: const Icon(Icons.undo_rounded, size: 20),
        ),
        const SizedBox(width: 6),
        IconButton(
          style: btnStyle,
          tooltip: 'Вперёд в истории',
          onPressed: state.isLoading || !state.canRedo
              ? null
              : () {
                  _flushDocumentToBloc();
                  context.read<EditorBloc>().add(const EditorRedo());
                },
          icon: const Icon(Icons.redo_rounded, size: 20),
        ),
      ],
    );
  }

  Widget _buildEditorPane(
    BuildContext context,
    EditorState state,
    ThemeData theme,
    ColorScheme colorScheme,
  ) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                padding: const EdgeInsets.all(8),
                decoration: BoxDecoration(
                  color: colorScheme.primaryContainer,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Icon(
                  Icons.subject_rounded,
                  size: 20,
                  color: colorScheme.primary,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  'Текст',
                  style: theme.textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
              if (_controller.text.isNotEmpty)
                IconButton(
                  icon: const Icon(Icons.copy_outlined, size: 22),
                  tooltip: 'Скопировать',
                  onPressed: () {
                    final text = _controller.text;
                    if (text.isEmpty) return;
                    Clipboard.setData(ClipboardData(text: text));
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(
                        content: const Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(Icons.check, color: Colors.white, size: 18),
                            SizedBox(width: 8),
                            Text('Скопировано'),
                          ],
                        ),
                        backgroundColor: colorScheme.primary,
                        behavior: SnackBarBehavior.floating,
                        duration: const Duration(seconds: 2),
                      ),
                    );
                  },
                ),
            ],
          ),
          const SizedBox(height: 12),
          Expanded(
            child: DecoratedBox(
              decoration: BoxDecoration(
                color: colorScheme.surfaceContainerHighest.withValues(
                  alpha: 0.35,
                ),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(
                  color: colorScheme.outline.withValues(alpha: 0.15),
                ),
              ),
              child: ClipRRect(
                borderRadius: BorderRadius.circular(12),
                child: TextField(
                  controller: _controller,
                  enabled: !state.isLoading,
                  maxLines: null,
                  expands: true,
                  textAlignVertical: TextAlignVertical.top,
                  style: theme.textTheme.bodyLarge?.copyWith(
                    height: 1.6,
                    fontSize: 15,
                  ),
                  decoration: InputDecoration(
                    hintText: state.isLoading
                        ? 'Обработка…'
                        : 'Введите или отредактируйте текст. Результат появится здесь же после Применить',
                    hintStyle: TextStyle(
                      color: colorScheme.onSurface.withValues(alpha: 0.45),
                    ),
                    border: InputBorder.none,
                    contentPadding: const EdgeInsets.all(16),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildModeSelector(
    BuildContext context,
    EditorState state, {
    bool embedded = false,
  }) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    return Container(
      decoration: _editorControlDecoration(colorScheme, embedded: embedded),
      child: DropdownButtonFormField<grpc.TransformType>(
        key: ValueKey(state.type),
        initialValue: state.type,
        isExpanded: true,
        isDense: true,
        decoration: const InputDecoration(
          border: InputBorder.none,
          contentPadding:
              EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          isDense: true,
        ),
        selectedItemBuilder: (context) {
          return _types.map((type) {
            final label = _labelForType(type);
            return Align(
              alignment: AlignmentDirectional.centerStart,
              child: Text(
                label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodyMedium,
              ),
            );
          }).toList();
        },
        items: _types
            .map(
              (type) => DropdownMenuItem(
                value: type,
                child: Text(
                  _labelForType(type),
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: theme.textTheme.bodyMedium,
                ),
              ),
            )
            .toList(),
        onChanged: state.isLoading
            ? null
            : (v) {
                if (v != null) {
                  context.read<EditorBloc>().add(EditorTypeChanged(v));
                }
              },
        dropdownColor: colorScheme.surface,
        borderRadius: BorderRadius.circular(8),
        iconSize: 20,
      ),
    );
  }

  Widget _buildModelSelector(
    BuildContext context,
    EditorState state, {
    bool embedded = false,
  }) {
    final theme = Theme.of(context);
    final colorScheme = theme.colorScheme;

    if (state.models.isEmpty) {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        alignment: Alignment.centerLeft,
        decoration: _editorControlDecoration(colorScheme, embedded: embedded),
        child: Text(
          'Нет моделей',
          style: theme.textTheme.bodyMedium?.copyWith(
            color: colorScheme.onSurface.withValues(alpha: 0.6),
          ),
        ),
      );
    }

    final modelValue =
        (state.selectedModel != null &&
                state.models.contains(state.selectedModel))
            ? state.selectedModel
            : state.models.first;

    return Tooltip(
      message: modelValue,
      waitDuration: const Duration(milliseconds: 500),
      child: Container(
        decoration: _editorControlDecoration(colorScheme, embedded: embedded),
        child: DropdownButtonFormField<String>(
          key: ValueKey(modelValue),
          initialValue: modelValue,
          isExpanded: true,
          isDense: true,
          decoration: const InputDecoration(
            border: InputBorder.none,
            contentPadding:
                EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            isDense: true,
          ),
          selectedItemBuilder: (context) {
            return state.models.map((m) {
              return Align(
                alignment: AlignmentDirectional.centerStart,
                child: Text(
                  m,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: theme.textTheme.bodyMedium,
                ),
              );
            }).toList();
          },
          items: state.models
              .map(
                (m) => DropdownMenuItem<String>(
                  value: m,
                  child: Text(
                    m,
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                    style: theme.textTheme.bodyMedium,
                  ),
                ),
              )
              .toList(),
          onChanged: state.isLoading || state.models.isEmpty
              ? null
              : (v) => context.read<EditorBloc>().add(EditorModelChanged(v)),
          dropdownColor: colorScheme.surface,
          borderRadius: BorderRadius.circular(8),
          iconSize: 20,
        ),
      ),
    );
  }

  Widget _buildMarkdownToggle(
    BuildContext context,
    EditorState state, {
    bool embedded = false,
  }) {
    final colorScheme = Theme.of(context).colorScheme;

    return ChoiceChip(
      visualDensity:
          embedded ? VisualDensity.compact : VisualDensity.standard,
      materialTapTargetSize: embedded
          ? MaterialTapTargetSize.shrinkWrap
          : MaterialTapTargetSize.padded,
      label: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.code,
            size: embedded ? 15 : 16,
            color: state.preserveMarkdown
                ? colorScheme.onPrimaryContainer
                : colorScheme.onSurface.withValues(alpha: 0.7),
          ),
          SizedBox(width: embedded ? 5 : 6),
          const Text('Markdown'),
        ],
      ),
      selected: state.preserveMarkdown,
      onSelected: state.isLoading
          ? null
          : (selected) {
              context.read<EditorBloc>().add(
                    EditorPreserveMarkdownChanged(selected),
                  );
            },
      selectedColor: colorScheme.primaryContainer,
      backgroundColor: embedded
          ? colorScheme.surface.withValues(alpha: 0.45)
          : null,
      side: embedded
          ? BorderSide(
              color: colorScheme.outline.withValues(alpha: 0.14),
            )
          : null,
      labelStyle: TextStyle(
        fontSize: embedded ? 13 : null,
        color: state.preserveMarkdown
            ? colorScheme.onPrimaryContainer
            : colorScheme.onSurface.withValues(alpha: 0.7),
        fontWeight:
            state.preserveMarkdown ? FontWeight.w600 : FontWeight.normal,
      ),
    );
  }

  Widget _buildPrimaryActions(
    BuildContext context,
    EditorState state,
    bool isMobile,
  ) {
    if (state.isLoading) {
      return FilledButton.tonalIcon(
        onPressed: () {
          context.read<EditorBloc>().add(const EditorCancelTransform());
        },
        icon: const Icon(Icons.stop_circle_outlined, size: 20),
        label: const Text(
          'Остановить',
          style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
        ),
        style: FilledButton.styleFrom(
          padding: EdgeInsets.symmetric(
            horizontal: isMobile ? 16 : 22,
            vertical: 14,
          ),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
        ),
      );
    }

    return FilledButton.icon(
      onPressed: () {
        _flushDocumentToBloc();
        context.read<EditorBloc>().add(const EditorTransformPressed());
      },
      icon: const Icon(Icons.auto_fix_high, size: 20),
      label: const Text(
        'Применить',
        style: TextStyle(
          fontWeight: FontWeight.w600,
          fontSize: 15,
        ),
      ),
      style: FilledButton.styleFrom(
        padding: EdgeInsets.symmetric(
          horizontal: isMobile ? 20 : 28,
          vertical: 16,
        ),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(12),
        ),
      ),
    );
  }
}
