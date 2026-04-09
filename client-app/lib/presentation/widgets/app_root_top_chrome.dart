import 'package:flutter/material.dart';

import 'package:gen/presentation/widgets/app_top_notice.dart';

class AppRootTopChrome extends StatelessWidget {
  const AppRootTopChrome({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return AppTopNoticeOverlay(child: child);
  }
}
