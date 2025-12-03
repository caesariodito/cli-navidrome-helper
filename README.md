# Navidrome Pixeldrain Importer (nd-import)

Small Go CLI that downloads a Pixeldrain archive (doubledouble.top/Pixeldrain links), cleans it, and merges the audio files into your Navidrome library under a chosen artist folder.

## Features
- Flags-only CLI: `--artist`, `--url`, `--tmp-dir`, `--keep-temp`, `--dry-run`.
- Reads `.env`/env vars (`NAVIDROME_MUSIC_PATH` required; `UNNEEDED_FILES`, `PIXELDRAIN_TOKEN` optional).
- Validates Pixeldrain URL/ID, streams download with optional token.
- Extracts zip to temp, prunes files via glob patterns (doublestar `**` supported) with “remove-all” safety guard.
- Detects destination collisions and aborts rather than overwriting existing files.
- Cleans temp download/extract dirs unless `--keep-temp`.

## Quick start
1) Copy `.env.example` to `.env` and set values:
   - `NAVIDROME_MUSIC_PATH=/absolute/path/to/navidrome/music`
   - `UNNEEDED_FILES=*.txt,*.nfo,Samples/**` (optional)
   - `PIXELDRAIN_TOKEN=...` (optional)
2) Run without installing:
```
go run ./cmd/nd-import --artist "Artist Name" --url https://pixeldrain.com/u/FILEID
# or positional form:
go run ./cmd/nd-import "Artist Name" https://pixeldrain.com/u/FILEID
```
3) Or build a binary:
```
go build -o nd-import ./cmd/nd-import
./nd-import --artist "Artist Name" --url FILEID_OR_URL
# or
./nd-import "Artist Name" FILEID_OR_URL
```

### Flags
- `--artist` (required): Artist folder name (sanitized to a safe path).
- `--url` (required): Pixeldrain URL or bare ID.
- `--tmp-dir`: Override temp base directory.
- `--keep-temp`: Leave download/extract dirs on disk.
- `--dry-run`: Validate and show actions; no writes to Navidrome path.

### Environment variables
- `NAVIDROME_MUSIC_PATH` (required): Absolute path to Navidrome music root.
- `UNNEEDED_FILES` (optional): Comma-separated globs to delete after extraction. If they would delete everything, the run aborts.
- `PIXELDRAIN_TOKEN` (optional): Bearer token if the link requires auth.

### Download progress
- The CLI displays a single-line progress indicator during download, showing transferred bytes, percent (when `Content-Length` is provided), speed, and ETA.
- After download completes, a newline is printed before further logs.

## Behavior notes
- Collision policy: aborts if any destination file/dir already exists under `${NAVIDROME_MUSIC_PATH}/${artist}`; nothing is overwritten.
- Download: requires the response to look like a zip (`Content-Type` containing `zip` or `octet-stream`), otherwise fails fast.
- Extraction: rejects absolute/parent-traversal paths inside zips.
- Pruning: uses `doublestar` patterns; directories matched by a pattern are removed recursively.
- Cleanup: temp dirs are removed after success/failure unless `--keep-temp`.

## Development
- Tests: `go test ./...`
- Lint/format: `gofmt -w .`
- CLI entrypoint: `cmd/nd-import/main.go`
- Core workflow: `internal/app/runner.go`
- Config loader: `internal/config/config.go`

## Assumptions and open questions
- Only zip archives are supported.
- Dest collisions abort; no overwrite/merge policy beyond “no conflicts”.
- Artist name is minimally sanitized (slashes -> `_`); no additional normalization.
