mpv native playback component

This directory contains a separate, unmodified mpv process. Hamster Toolbox
launches it for local playback and controls it through local JSON IPC.
The toolbox does not link libmpv into its MIT-licensed executable.

Binary: shinchiro Windows x86_64 build, listed on the mpv installation page.
Build/version: mpv v0.42.0-38-gbcde6b93c
Fixed build tag: https://github.com/shinchiro/mpv-winbuild-cmake/releases/tag/20261111
Archive: mpv-x86_64-20261111-git-bcde6b93cc.7z
Archive SHA256: f259b5fd4b5c395617c948a4fadcdf55bba1c5282fb7c13e88626b3f71072baf
The archive digest was verified against the publisher's GitHub release asset.
Source: https://github.com/mpv-player/mpv/tree/bcde6b93cca4d128d84e1a60d1a6db2cd81c889e
Build recipes: https://github.com/shinchiro/mpv-winbuild-cmake/tree/20261111
FFmpeg reported by this build: N-128766-gcd4e96eb3
FFmpeg source: https://github.com/FFmpeg/FFmpeg/tree/cd4e96eb3
libplacebo reported by this build: v7.391.0 (v7.387.0-37-gc2cf6e7-dirty)
libplacebo source: https://code.videolan.org/videolan/libplacebo
Installation/build information: https://mpv.io/installation/

The general x86_64 archive is used, not the separate x86_64-v3 archive.
Installer, file-association and updater scripts are not bundled or run.
Upstream Copyright, LICENSE.GPL and LICENSE.LGPL accompany this component.
mpv and its linked dependencies retain their upstream licenses; they are not
relicensed under the toolbox's MIT license. Consult the upstream build recipes
and copyright notice for dependency-specific licenses and source locations.
