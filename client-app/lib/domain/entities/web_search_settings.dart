class WebSearchSettingsEntity {
  const WebSearchSettingsEntity({
    required this.enabled,
    required this.maxResults,
    required this.braveApiKey,
    required this.googleApiKey,
    required this.googleSearchEngineId,
    required this.yandexUser,
    required this.yandexKey,
    required this.yandexEnabled,
    required this.googleEnabled,
    required this.braveEnabled,
  });

  final bool enabled;
  final int maxResults;
  final String braveApiKey;
  final String googleApiKey;
  final String googleSearchEngineId;
  final String yandexUser;
  final String yandexKey;
  final bool yandexEnabled;
  final bool googleEnabled;
  final bool braveEnabled;
}
