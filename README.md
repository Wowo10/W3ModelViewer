# modelViewer

A Go tool for exploring a local Warcraft III game archive (CASC) and viewing
its `.mdx` models. Each model can be shown in its classic, Reforged and
definitive-edition versions, with textures, team colour and animation, in a
desktop window.

The game data stays on your machine: nothing from it is committed.

## AI disclaimer

AI tools may have been used to assist with writing code and documentation in this
project. AI-generated content can be inaccurate or incomplete. The project is provided as-is; users are
responsible for evaluating its behavior and complying with applicable licenses.

## Download

Prebuilt binaries for Linux (x86-64), Windows (x86-64) and macOS (Apple
Silicon and Intel) are on the
[releases page](https://github.com/Wowo10/W3ModelViewer/releases). SDL2 is
built in; you only need OpenGL 3.2 drivers (and, on Linux, a distribution
from 2024 or later, such as Ubuntu 24.04 or Debian 13). Unpack the archive and run
`modelViewer` (`modelViewer.exe` on Windows): it asks for the Warcraft III
folder on first start and saves it in `modelViewer.json` next to the binary.

The macOS binary isn't signed, so macOS blocks it at first; allow it once
with `xattr -d com.apple.quarantine modelViewer`.

## Requirements

- Go 1.27 or newer.
- SDL2 and an OpenGL 3.2 driver for the desktop viewer (the Go bindings use
  cgo, so a C compiler too).
- A Warcraft III installation (the directory with `.build.info` and
  `Data/`). The desktop viewer asks for it on first start and saves it in
  `modelViewer.json` next to the binary (or the working directory, under
  `go run`). Output flags and the
  tests read a copy or symlink of it at `wardata/` in the repository root.

## Running

```sh
make run                    # desktop viewer
make run ARGS=-h            # every flag
```

`go run .` also works, but builds into a temporary directory, so
`modelViewer.json` ends up in the working directory instead of next to the
binary; `make run` keeps both in the repository root.

**Desktop viewer.** On first start (or with no valid install configured) a
setup window asks for the Warcraft III install folder: type a path or use
"Browse..." (the system's folder picker, or a built-in one when none is
available). Once it's saved, pick a model in the directory tree (with a text
filter and an edition select). Models with no geometry (particle effects and the
like) are hidden by default; a "show models without geometry" checkbox
brings them back, each with a short explanation of what it holds instead.
A background scan builds this the first time (progress shown in the Models
window) and caches it under `os.UserCacheDir()/modelViewer/` for the next
start. The Model window shows its vertex and triangle counts per geoset, its
material layers and buttons for the asset's other editions. The Animation
window plays its sequences. Drag to rotate, scroll to zoom.

**Command-line output.** Instead of opening the viewer:

| flag | prints |
|------|--------|
| `-list` | every file in the archive: path, size, content key, edition, overlays (narrow with `-filter '<glob>'` and `-edition classic\|reforged\|definitive\|install`) |
| `-variants <path>` | every edition and overlay of one file |
| `-extract <full path>` | writes one file to `-out` (default `.`) |
| `-mdxinfo <full path>` | a model's chunks, textures, materials, sequences, geoset counts and skinning summary |
| `-survey` | reads every file once and reports what decodes |

Full paths include the mod chain, e.g.
`War3.w3mod:_HD.w3mod:Units/Human/Footman/Footman.mdx`; `-variants` lists
them. `-verify=false` skips checking the index against the archives at
startup (faster).

## How the archive is read

A file's path leads to its bytes in four steps:

1. The **root** file maps a path (with its mod chain: base `War3.w3mod` is
   classic, `_HD.w3mod` Reforged, `_DE.w3mod` definitive) to a content key,
   the MD5 of the file's contents.
2. The **encoding** file maps a content key to an encoding key, the MD5 of
   the stored, compressed form.
3. The **`.idx`** files map an encoding key to an archive (`data.###`), an
   offset and a size.
4. The bytes there are **BLTE**: chunks that are raw, zlib-compressed or
   nested. Decoding them gives the original file.

`.build.info` and the build config name the encoding and root files to start
from.

## Layout

| path | what |
|------|------|
| `extract/` | CASC: build info, build config, `.idx`, BLTE, encoding, root, `casc.Storage` |
| `model/mdx` | MDX parsing (geosets, materials, textures, sequences, bones, skinning) and pose evaluation |
| `model/texture` | texture path resolution, DDS headers, drawable layers |
| `model/catalog` | background scan for geometry/effect counts per model, cached on disk |
| `settings/` | `modelViewer.json` (install path) location, loading/saving and install folder validation |
| `viewer/` | desktop viewer (SDL2, OpenGL 3.2, Dear ImGui via cimgui-go) |
| `main.go` | flags and command-line output |

## Development

```sh
gofmt -l . && go vet ./... && go build ./...
go test ./...                  # some tests read wardata/
```

**Releases.** Pushing a `v*` tag runs `.github/workflows/release.yml`: it
builds the three platforms on GitHub's runners, with SDL2 compiled from
source and linked statically, and publishes a release with the archives
attached. Running the workflow by hand from the Actions tab
builds the same archives as downloadable artifacts, without a release.

## License

[MIT](LICENSE). Warcraft III game data is not covered and is never part of
this repository.
