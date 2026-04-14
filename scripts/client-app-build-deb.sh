#!/usr/bin/env bash
set -e

BUNDLE_DIR="${1:-client-app/build/linux/x64/release/bundle}"
OUTPUT_DEB="${2:-gen-amd64.deb}"

if [ -n "$VERSION" ]; then
  :
elif [ -n "$RELEASE_TAG" ] && [[ "$RELEASE_TAG" =~ ^v[0-9] ]]; then
  VERSION="${RELEASE_TAG#v}"
else
  VERSION=$(grep '^version:' client-app/pubspec.yaml | sed 's/version: *\([0-9.]*\).*/\1/')
fi
DEB_NAME="gen-${VERSION}-amd64.deb"

PKG_DIR="deb_staging"
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/DEBIAN"
mkdir -p "$PKG_DIR/opt/gen"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/usr/share/applications"
mkdir -p "$PKG_DIR/usr/share/pixmaps"

cp -a "$BUNDLE_DIR"/* "$PKG_DIR/opt/gen"

ICON_SRC="client-app/linux/runner/resources/app_icon.png"
if [ -f "$ICON_SRC" ]; then
  cp "$ICON_SRC" "$PKG_DIR/usr/share/pixmaps/gen.png"
else
  echo "Warning: Icon file not found at $ICON_SRC"
fi

cat > "$PKG_DIR/usr/bin/gen" << 'WRAPPER'
#!/bin/sh
exec /opt/gen/gen "$@"
WRAPPER
chmod 755 "$PKG_DIR/usr/bin/gen"

cat > "$PKG_DIR/usr/share/applications/gen.desktop" << 'DESKTOP'
[Desktop Entry]
Name=Gen
Comment=Gen desktop application
Exec=/opt/gen/gen
Icon=gen
Terminal=false
Type=Application
Categories=Utility;
DESKTOP

cat > "$PKG_DIR/DEBIAN/control" << CONTROL
Package: gen
Version: $VERSION
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Gen <info@magomedcoder.ru>
Description: Gen desktop application
CONTROL

dpkg-deb -b "$PKG_DIR" "$DEB_NAME"
rm -rf "$PKG_DIR"

if [ -n "$OUTPUT_DEB" ] && [ "$DEB_NAME" != "$OUTPUT_DEB" ]; then
  mv "$DEB_NAME" "$OUTPUT_DEB"
fi

echo "Built: $DEB_NAME"
