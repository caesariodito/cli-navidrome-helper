# Navidrome Pixeldrain Importer PRD

## Goal
Build a small Go CLI that downloads music archives from Pixeldrain (doubledouble.top links), cleans them, and places the sanitized audio files into the Navidrome library under the correct artist folder.

## Users & Use Cases
- Navidrome admins who want a one-step flow to pull Pixeldrain zips and drop them into their library.
- Run ad hoc from the shell or as part of a simple automation script.

## Functional Requirements
- **CLI entrypoint:** Single binary command (e.g., `nd-import`) with subcommand or flags to import an archive.
  - Required flag: `--artist` (artist folder name to group tracks).
  - Required flag: `--url` (Pixeldrain download URL; accepting either full URL or ID if easily derivable).
  - Optional flags: `--tmp-dir` override, `--keep-temp` to skip cleanup, `--dry-run` for validation-only.
- **Environment configuration:** Read from `.env` when present.
  - `NAVIDROME_MUSIC_PATH` (required): absolute path to Navidrome music root.
  - `UNNEEDED_FILES` (optional): comma-separated list of glob patterns to remove post-extraction (e.g., `*.txt,*.jpg,Samples/**`).
  - `PIXELDRAIN_TOKEN` (optional) if authenticated downloads are needed.
- **Download behavior:**
  - Validate URL/ID format and fail fast on missing/invalid input.
  - Stream download to a temp location; show progress if feasible.
  - Detect obvious content-type/extension mismatch and fail with a clear message.
- **Archive handling:**
  - Extract zip into an isolated temp directory.
  - Remove files matching `UNNEEDED_FILES` patterns before moving to destination.
  - Keep only audio-relevant content (config-driven; do not hardcode deletions beyond env patterns).
- **Placement into Navidrome:**
  - Destination path: `${NAVIDROME_MUSIC_PATH}/${artist}`.
  - Create artist folder if absent.
  - If destination already exists, support strategies: merge when no conflicts, or abort with a helpful error if collisions occur. Do not overwrite files silently.
- **Cleanup & artifacts:**
  - Remove temp download and extraction directories unless `--keep-temp` is set.
  - Provide a `--dry-run` mode to show intended operations without writing.
- **Logging & UX:**
  - Human-friendly status logs to stdout; errors to stderr with non-zero exit codes.
  - Summarize actions (download path, extracted files count, removed patterns, final destination).

## Non-Functional Requirements
- Written in Go (target current stable; assume Go 1.22+).
- Works on Linux; avoid platform-specific temp-path handling.
- Deterministic and repeatable: running twice with identical inputs should not produce duplicate files or partial state.
- Avoid leaving temp files on failure where possible.
- Minimal external dependencies; prefer stdlib unless a library meaningfully reduces risk (e.g., Cobra for CLI ergonomics, dotenv for env loading).
- Tests for core behaviors (URL validation, pattern removal, move/merge logic, dry-run safeguards).

## Edge Cases & Error Handling
- Missing `NAVIDROME_MUSIC_PATH` or invalid path (non-existent, not a directory).
- Invalid Pixeldrain URL/ID, network errors, or HTTP non-200 responses.
- Non-zip response or corrupted archive (checksum/zip read errors).
- `UNNEEDED_FILES` patterns that match everything (should warn and abort).
- Destination collision (same filename); configurable policy: abort with clear message by default.
- Insufficient disk space or permissions in temp or destination path (surface actionable errors).

## Out of Scope
- Automated artist metadata tagging or audio normalization.
- Download queues or background daemonization.
- Support for archive formats beyond zip (unless trivial to add).
- Navidrome API interactions beyond file placement.

## Open Questions
1) Should we support multiple downloads in one command (batch input file or multiple URLs)?  
2) Preferred collision policy: abort on any conflict, overwrite identical sizes, or prompt?  
3) Any default `UNNEEDED_FILES` patterns to bake in (e.g., cover art, NFO/URL files), or strictly user-provided?  
4) Should we sanitize artist names (e.g., replace `/` or path-unsafe characters), and what mapping rules are acceptable?  
5) Should we allow archive formats beyond zip (rar/7z) given future needs?  
6) Is authenticated Pixeldrain access required, and how should credentials be supplied (token vs. cookie)?  
