import 'package:gen/domain/repositories/runners_repository.dart';

class GetWebSearchAvailabilityUseCase {
  final RunnersRepository _repo;

  GetWebSearchAvailabilityUseCase(this._repo);

  Future<bool> call() async {
    try {
      return await _repo.getWebSearchGloballyEnabled();
    } catch (_) {
      return false;
    }
  }
}
