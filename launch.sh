#!/bin/sh
PAK_DIR="$(dirname "$0")"
PAK_NAME="$(basename "$PAK_DIR")"
PAK_NAME="${PAK_NAME%.*}"
export HOME="$SHARED_USERDATA_PATH/$PAK_NAME"
export LD_LIBRARY_PATH="$PAK_DIR/lib:$LD_LIBRARY_PATH"
export PATH="$PAK_DIR:$PATH"
mkdir -p "$HOME"
exec "$PAK_DIR/itchio-pak"
