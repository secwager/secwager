## Purpose

This repository is a minimal C++ example that builds a single executable (`HelloWorld`) with CMake and consumes the `fmt` library through vcpkg manifest-mode. The instructions below give an AI coding agent the essential context and precise commands to be productive immediately.

## Big picture

- Single-target command-line program: `add_executable(HelloWorld helloworld.cpp)` in `CMakeLists.txt`.
- External dependency managed by vcpkg manifest: `vcpkg.json` lists `fmt` and CMake links it as `fmt::fmt`.
- CMakePresets (`CMakePresets.json` / `CMakeUserPresets.json`) configure the Ninja generator and set `CMAKE_TOOLCHAIN_FILE` to use vcpkg.

Key files to inspect:
- `CMakeLists.txt` — project, target and linking pattern (see `target_link_libraries(HelloWorld PRIVATE fmt::fmt)`).
- `CMakePresets.json` / `CMakeUserPresets.json` — configure presets; the `default` preset inherits `vcpkg` and sets `VCPKG_ROOT`.
- `vcpkg.json` — manifest dependencies. Add any new library here and run `vcpkg` to restore.
- `helloworld.cpp` — example usage of `fmt::print`.

## Build / run / debug (concrete examples)

Preferred (uses presets):

```bash
# ensure VCPKG_ROOT points to an installed vcpkg clone; CMakeUserPresets.json defaults to ~/vcpkg
export VCPKG_ROOT=~/vcpkg
cmake --preset default        # configure using the preset that uses vcpkg + Ninja
cmake --build build          # build the HelloWorld target (Ninja)
./build/HelloWorld            # run the binary
```

Alternative explicit configure (if you don't use presets):

```bash
mkdir -p build
cmake -S . -B build -G Ninja -DCMAKE_TOOLCHAIN_FILE="$VCPKG_ROOT/scripts/buildsystems/vcpkg.cmake"
cmake --build build
./build/HelloWorld
```

Debugging notes:
- The project is built with Ninja; run the executable under `gdb` or your IDE. Example: `gdb --args ./build/HelloWorld`.
- VS Code users: the CMake Tools extension can pick up `CMakePresets.json`; use the `default` configure preset.

## Project-specific conventions and patterns

- Dependencies are managed via vcpkg manifest (`vcpkg.json`). Add packages to `vcpkg.json` and run `vcpkg install` (or let CMake configure drive a manifest-mode restore). Do not add libraries directly to `CMakeLists.txt` — prefer manifest-first.
- Link external libraries using imported targets (e.g., `fmt::fmt`) and `PRIVATE` linkage for binaries.
- Keep single-file example code at repo root (`helloworld.cpp`) — follow same placement for small examples or tests.

## Integration points / external dependencies

- vcpkg: watch `VCPKG_ROOT` in `CMakeUserPresets.json`. CI or dev machines should set `VCPKG_ROOT` to a vcpkg clone or use a global installation.
- CMake presets use Ninja as generator; expect `ninja` to be available on developer machines or CI images.

## When modifying the repo

- If you add dependencies: update `vcpkg.json` and reconfigure (CMake configure will pick up the manifest). Example: add `dependencies: ["fmt", "boost-filesystem"]`.
- If you add targets: follow the `target_link_libraries(<target> PRIVATE <pkg>::<target>)` pattern.

## Quick pointers for the AI agent

- For build-related tasks, prefer using `cmake --preset default` so vcpkg toolchain and generator are applied consistently.
- Inspect `CMakeLists.txt` for target names before making edits — the repo uses `HelloWorld` as the canonical target.
- Verify changes locally by running the `cmake` configure + `cmake --build` sequence and executing `./build/HelloWorld`.

---

If any of these paths or presets are different on your machine, tell me where to look and I will update this guidance. Ready to adjust if you want extra sections (CI, tests, or more examples).
