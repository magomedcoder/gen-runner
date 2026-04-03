import 'package:equatable/equatable.dart';
import 'package:gen/domain/entities/runner_info.dart';

class RunnersAdminState extends Equatable {
  static const Object _noChange = Object();

  final bool isLoading;
  final List<RunnerInfo> runners;
  final String? defaultRunner;
  final String? error;

  const RunnersAdminState({
    this.isLoading = false,
    this.runners = const [],
    this.defaultRunner,
    this.error,
  });

  RunnersAdminState copyWith({
    bool? isLoading,
    List<RunnerInfo>? runners,
    Object? defaultRunner = _noChange,
    Object? error = _noChange,
  }) {
    return RunnersAdminState(
      isLoading: isLoading ?? this.isLoading,
      runners: runners ?? this.runners,
      defaultRunner: identical(defaultRunner, _noChange)
          ? this.defaultRunner
          : defaultRunner as String?,
      error: identical(error, _noChange) ? this.error : error as String?,
    );
  }

  @override
  List<Object?> get props => [
    isLoading,
    runners,
    defaultRunner,
    error,
  ];
}
