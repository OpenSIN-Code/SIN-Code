#!/usr/bin/env bash
# Purpose: Generate platform icons (PNG, ICNS, ICO) from icon.svg
# Docs: scripts/build-icons.doc.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ICON_DIR="$SCRIPT_DIR/../src-tauri/icons"
SVG="$ICON_DIR/icon.svg"

if [ ! -f "$SVG" ]; then
    echo "ERROR: $SVG not found"
    exit 1
fi

echo "Generating icons from $SVG..."

# Generate PNGs at various sizes (Tauri needs 32, 128, 256, 512)
for size in 32 128 256 512; do
    if command -v rsvg-convert &> /dev/null; then
        rsvg-convert -w "$size" -h "$size" "$SVG" -o "$ICON_DIR/icon-${size}.png"
        echo "  -> icon-${size}.png (${size}x${size})"
    elif command -v magick &> /dev/null; then
        magick -background none -density 300 "$SVG" -resize "${size}x${size}" "$ICON_DIR/icon-${size}.png"
        echo "  -> icon-${size}.png (${size}x${size})"
    elif command -v sips &> /dev/null; then
        # sips needs raster input, so use a temp PNG first
        sips -z "$size" "$size" "$SVG" --out "$ICON_DIR/icon-${size}.png" 2>/dev/null || true
    else
        echo "ERROR: install librsvg (rsvg-convert) or ImageMagick (magick)"
        exit 1
    fi
done

# Default icon.png = 512x512
cp "$ICON_DIR/icon-512.png" "$ICON_DIR/icon.png"

# Generate tray icon (small monochrome)
if command -v rsvg-convert &> /dev/null; then
    rsvg-convert -w 32 -h 32 "$SVG" -o "$ICON_DIR/tray-icon.png"
elif command -v magick &> /dev/null; then
    magick -background none -density 300 "$SVG" -resize "32x32" "$ICON_DIR/tray-icon.png"
fi

# macOS .icns
if command -v iconutil &> /dev/null; then
    ICONSET="$ICON_DIR/icon.iconset"
    mkdir -p "$ICONSET"
    cp "$ICON_DIR/icon-32.png" "$ICONSET/icon_16x16.png"
    cp "$ICON_DIR/icon-32.png" "$ICONSET/icon_32x32.png"
    cp "$ICON_DIR/icon-128.png" "$ICONSET/icon_128x128.png"
    cp "$ICON_DIR/icon-128.png" "$ICONSET/icon_128x128@2x.png"
    cp "$ICON_DIR/icon-256.png" "$ICONSET/icon_256x256.png"
    cp "$ICON_DIR/icon-256.png" "$ICONSET/icon_256x256@2x.png"
    cp "$ICON_DIR/icon-512.png" "$ICONSET/icon_512x512.png"
    cp "$ICON_DIR/icon-512.png" "$ICONSET/icon_512x512@2x.png"
    iconutil -c icns "$ICONSET" -o "$ICON_DIR/icon.icns"
    rm -rf "$ICONSET"
    echo "  -> icon.icns"
fi

# Windows .ico
if command -v magick &> /dev/null; then
    magick "$ICON_DIR/icon-16.png" "$ICON_DIR/icon-32.png" "$ICON_DIR/icon-48.png" \
          "$ICON_DIR/icon-64.png" "$ICON_DIR/icon-128.png" "$ICON_DIR/icon-256.png" \
          "$ICON_DIR/icon.ico"
    echo "  -> icon.ico"
fi

echo "Done. Icons in $ICON_DIR:"
ls -la "$ICON_DIR"