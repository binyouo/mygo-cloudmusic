#!/bin/sh
set -efu

# MyGo sets CC to clang with the target and SDK of this specific build.
# Keep its target and SDK when compiling and linking C++.
exec ${CC:?MyGo must provide the iOS clang command} --driver-mode=g++ "$@"
