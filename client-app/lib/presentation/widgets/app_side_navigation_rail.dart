import 'dart:math' as math;

import 'package:flutter/material.dart';

class AppSideNavDestination {
  const AppSideNavDestination({
    required this.icon,
    required this.selectedIcon,
    required this.label,
    this.alignBottom = false,
  });

  final IconData icon;
  final IconData selectedIcon;
  final String label;
  final bool alignBottom;
}

class AppSideNavigationRail extends StatelessWidget {
  const AppSideNavigationRail({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.destinations,
    this.indicatorSize = 48,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final List<AppSideNavDestination> destinations;

  final double indicatorSize;

  static double _itemExtent(double indicatorSide) {
    return math.max(indicatorSide + 12, 52);
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final navTheme = theme.navigationRailTheme;
    final scheme = theme.colorScheme;
    final bg = navTheme.backgroundColor ?? scheme.surfaceContainerLow;
    final indicatorColor = navTheme.indicatorColor;
    final selectedIconTheme = navTheme.selectedIconTheme ?? const IconThemeData.fallback();
    final unselectedIconTheme = navTheme.unselectedIconTheme ?? const IconThemeData.fallback();
    final railWidth = navTheme.minWidth ?? 80.0;
    final itemH = _itemExtent(indicatorSize);
    final indexed = List.generate(destinations.length, (i) => (
      index: i,
      destination: destinations[i]
    ));
    final topItems = indexed.where((e) => !e.destination.alignBottom).toList();
    final bottomItems = indexed.where((e) => e.destination.alignBottom).toList();

    return Material(
      color: bg,
      child: SizedBox(
        width: railWidth,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            const SizedBox(height: 8),
            for (int i = 0; i < topItems.length; i++) ...[
              _SideNavItem(
                selected: selectedIndex == topItems[i].index,
                destination: topItems[i].destination,
                itemHeight: itemH,
                indicatorSize: indicatorSize,
                indicatorColor: indicatorColor,
                selectedIconTheme: selectedIconTheme,
                unselectedIconTheme: unselectedIconTheme,
                onTap: () => onDestinationSelected(topItems[i].index),
              ),
              if (i < topItems.length - 1) const SizedBox(height: 4),
            ],
            const Spacer(),
            for (int i = 0; i < bottomItems.length; i++) ...[
              _SideNavItem(
                selected: selectedIndex == bottomItems[i].index,
                destination: bottomItems[i].destination,
                itemHeight: itemH,
                indicatorSize: indicatorSize,
                indicatorColor: indicatorColor,
                selectedIconTheme: selectedIconTheme,
                unselectedIconTheme: unselectedIconTheme,
                onTap: () => onDestinationSelected(bottomItems[i].index),
              ),
              if (i < bottomItems.length - 1) const SizedBox(height: 4),
            ],
            if (bottomItems.isNotEmpty) const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}

class _SideNavItem extends StatefulWidget {
  const _SideNavItem({
    required this.selected,
    required this.destination,
    required this.itemHeight,
    required this.indicatorSize,
    required this.indicatorColor,
    required this.selectedIconTheme,
    required this.unselectedIconTheme,
    required this.onTap,
  });

  final bool selected;
  final AppSideNavDestination destination;
  final double itemHeight;
  final double indicatorSize;
  final Color? indicatorColor;
  final IconThemeData selectedIconTheme;
  final IconThemeData unselectedIconTheme;
  final VoidCallback onTap;

  @override
  State<_SideNavItem> createState() => _SideNavItemState();
}

class _SideNavItemState extends State<_SideNavItem> {
  bool _hover = false;

  static const double _indicatorRadius = 12;
  static const Duration _hoverAnim = Duration(milliseconds: 120);
  static const double _hoverUnselectedAlpha = 0.08;
  static const double _hoverSelectedWhiteBlend = 0.012;

  Color? _indicatorFill() {
    final c = widget.indicatorColor;
    if (c == null) {
      return null;
    }
    if (widget.selected) {
      if (_hover) {
        return Color.alphaBlend(
          Theme.of(context).colorScheme.onSurface.withValues(
            alpha: _hoverSelectedWhiteBlend,
          ),
          c,
        );
      }
      return c;
    }
    if (_hover) {
      return c.withValues(alpha: _hoverUnselectedAlpha);
    }
    return null;
  }

  @override
  Widget build(BuildContext context) {
    final iconData = widget.selected ? widget.destination.selectedIcon : widget.destination.icon;
    final iconTheme = widget.selected ? widget.selectedIconTheme : widget.unselectedIconTheme;
    final fill = _indicatorFill();

    return Tooltip(
      message: widget.destination.label,
      child: Semantics(
        button: true,
        selected: widget.selected,
        label: widget.destination.label,
        child: MouseRegion(
          onEnter: (_) => setState(() => _hover = true),
          onExit: (_) => setState(() => _hover = false),
          cursor: SystemMouseCursors.click,
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: widget.onTap,
            child: SizedBox(
              width: double.infinity,
              height: widget.itemHeight,
              child: Center(
                child: Stack(
                  alignment: Alignment.center,
                  clipBehavior: Clip.none,
                  children: [
                    if (fill != null)
                      AnimatedContainer(
                        duration: _hoverAnim,
                        curve: Curves.easeOut,
                        width: widget.indicatorSize,
                        height: widget.indicatorSize,
                        decoration: ShapeDecoration(
                          color: fill,
                          shape: RoundedRectangleBorder(
                            borderRadius:
                                BorderRadius.circular(_indicatorRadius),
                          ),
                        ),
                      ),
                    IconTheme(
                      data: iconTheme,
                      child: Icon(iconData, size: iconTheme.size ?? 24),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
