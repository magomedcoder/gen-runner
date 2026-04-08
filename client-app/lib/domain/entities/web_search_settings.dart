class WebSearchSettingsEntity {
  const WebSearchSettingsEntity({
    required this.enabled,
    required this.maxResults,
    required this.braveApiKey,
    required this.googleApiKey,
    required this.googleSearchEngineId,
    required this.yandexUser,
    required this.yandexKey,
  });

  final bool enabled;
  final int maxResults;
  final String braveApiKey;
  final String googleApiKey;
  final String googleSearchEngineId;
  final String yandexUser;
  final String yandexKey;
}
