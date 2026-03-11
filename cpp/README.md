# Experimental C++ Port

This directory is a Linux-only sandbox for porting selected `UCXSync` components to C++.

## Current scope

- experimental only;
- not used by the production Go application;
- focused on porting the configuration layer first.

## Goals

1. mirror the Go configuration model;
2. keep Linux deployment assumptions explicit;
3. isolate platform-specific code from business logic;
4. avoid changing the production Go runtime while porting.

## Current contents

- `CMakeLists.txt` — standalone build file for the experimental port
- `include/config.hpp` — public configuration model and API
- `src/config/config.cpp` — YAML loading, defaults, validation
- `src/main.cpp` — tiny smoke-test style entry point

## Build prerequisites

- Linux
- CMake 3.16+
- C++17 compiler
- network access during configure step (for `yaml-cpp` via `FetchContent`)

## Build

```bash
cd cpp
cmake -S . -B build
cmake --build build
./build/ucxsync_cpp
```

## Design notes

- The C++ port follows the current Go config shape, not a future idealized one.
- Mount root configuration is still hard-coded in the Go runtime; the port documents this instead of pretending otherwise.
- The next reasonable step after config is a small Linux-only filesystem abstraction for mount discovery.
