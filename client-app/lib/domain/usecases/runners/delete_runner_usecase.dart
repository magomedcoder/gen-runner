import 'package:gen/domain/repositories/runners_repository.dart';

class DeleteRunnerUseCase {
  final RunnersRepository _repo;

  DeleteRunnerUseCase(this._repo);

  Future<void> call(int id) => _repo.deleteRunner(id);
}
