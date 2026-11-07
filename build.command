#!/bin/bash
# Double-click in Finder to build "Smart Cal.app" onto the Desktop.
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$HOME/.homebrew/bin:$HOME/go/bin:$PATH"
cd "$(dirname "$0")"
{
  echo "== $(date) =="; which go; go version; xcode-select -p
  ./build.sh --run
  echo "EXIT: $?"
} 2>&1 | tee build.log
