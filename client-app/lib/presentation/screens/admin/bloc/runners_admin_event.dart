import 'package:equatable/equatable.dart';

sealed class RunnersAdminEvent extends Equatable {
  const RunnersAdminEvent();

  @override
  List<Object?> get props => [];
}

class RunnersAdminLoadRequested extends RunnersAdminEvent {
  const RunnersAdminLoadRequested();
}

class RunnersAdminCreateRequested extends RunnersAdminEvent {
  final String name;
  final String host;
  final int port;
  final bool enabled;
  final String selectedModel;

  const RunnersAdminCreateRequested({
    required this.name,
    required this.host,
    required this.port,
    required this.enabled,
    this.selectedModel = '',
  });

  @override
  List<Object?> get props => [name, host, port, enabled, selectedModel];
}

class RunnersAdminUpdateRequested extends RunnersAdminEvent {
  final int id;
  final String name;
  final String host;
  final int port;
  final bool enabled;
  final String selectedModel;

  const RunnersAdminUpdateRequested({
    required this.id,
    required this.name,
    required this.host,
    required this.port,
    required this.enabled,
    this.selectedModel = '',
  });

  @override
  List<Object?> get props => [id, name, host, port, enabled, selectedModel];
}

class RunnersAdminDeleteRequested extends RunnersAdminEvent {
  final int id;

  const RunnersAdminDeleteRequested(this.id);

  @override
  List<Object?> get props => [id];
}

class RunnersAdminClearError extends RunnersAdminEvent {
  const RunnersAdminClearError();
}

class RunnersAdminDefaultRunnerChanged extends RunnersAdminEvent {
  final String address;

  const RunnersAdminDefaultRunnerChanged(this.address);

  @override
  List<Object?> get props => [address];
}
