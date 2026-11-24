# Architecture

The code uses a hexagonal architecture (ports and adapters), laid out with Go's usual `cmd/` + `internal/` folders.

```
cmd/mimo-ytdownloader/main.go                  wiring: config -> adapters -> use case -> CLI
internal/
  domain/                           core rules, no dependencies (stdlib only, no I/O)
    job.go, outcome.go, summary.go  Job, Fetched, Status, Outcome, Summary
    duplicate.go, videokey.go       one download per video: streaming Deduper, YouTubeID
    playlist.go                     IsPlaylist: which links get expanded
    search.go                       IsSearchPage: search pages are rejected
    errors.go                       ErrTransient / ErrPermanent / ErrInvalidInput
  application/                      depends on domain
    model/policy.go                 DownloadPolicy
    port/in/download_batch.go       DownloadBatchUseCase  (input port)
    port/out/audio_downloader.go    AudioDownloader       (output port)
    port/out/playlist_resolver.go   PlaylistResolver      (output port)
    port/out/progress_reporter.go   ProgressReporter      (output port)
    usecase/download_batch.go       DownloadBatch implements DownloadBatchUseCase
  infrastructure/                   depends on application and domain
    adapter/in/cli/                 flags, streaming input, song CSV enrichment, Controller, exit codes
    adapter/out/ytdlp/              AudioDownloader + PlaylistResolver + SongResolver using yt-dlp
    adapter/out/console/            ProgressReporter on stderr
    config/                         YAML -> Config, DownloadPolicy, ytdlp.Properties
  architecture/                     test that enforces the dependency rules
deploy/                             container config and the AWS ECS task definition
Dockerfile, compose.yaml            container image and local Docker Compose setup
```

## Rules

- **Dependencies point inward:** `infrastructure -> application -> domain`. The `internal/architecture` test checks this.
- **`domain` and `application`** never import `os`, `os/exec`, `net`, YAML, or `infrastructure`.
- **Interfaces belong to the code that uses them** (`application/port/out`). Adapters implement them and check it at compile time: `var _ portout.AudioDownloader = (*Downloader)(nil)`.
- **Errors:** adapters wrap their failures in domain errors (`fmt.Errorf("%w: ...", domain.ErrTransient)`). The use case decides with `errors.Is` and knows nothing about yt-dlp.
- **`main`** only wires things together. No business logic.

## Flow

1. `cli.ParseFlags` reads the flags.
2. `config.Load` reads and validates the YAML.
3. `ytdlp.New` loads the archive into memory. `Preflight` checks that yt-dlp and ffmpeg work.
4. For a CSV with `artist` and `title`, `cli.EnrichCSV` searches for studio videos, inspects up to three candidates per song, reports each result and retry, and atomically adds missing URLs. Unmatched songs count as failed tracks.
5. `usecase.NewDownloadBatch` builds the use case. `playlists` is `nil` when `expand_playlists: false`.
6. `cli.Controller.Handle` streams the input (`iter.Seq2[Job, error]`) into `Execute`, which runs this pipeline:

```
read ──► (playlist? ──► resolvers ×W) ──► dedupe ──► downloaders ×W ──► collect
  │                         │                               │
  └── bad rows ─────────────┴── failed playlists ───────────┴──► results
```

7. `cli.ExitCode` turns the summary into an exit code.

- **Bounded channels:** a slow stage slows down the ones before it, so memory does not depend on input size.
- **Dedupe:** a single goroutine owns the `Deduper`, so no locks are needed.
- **One lock for stderr:** progress lines and the Ctrl-C notice come from different goroutines, so `main` wraps stderr in a small `syncWriter`.
- **Bad rows** are reported as `FAIL` and the batch continues. A read error (I/O) stops reading and is returned with the summary.

## Cancellation

- **`signal.NotifyContext`:** the first Ctrl-C (or SIGTERM) cancels the context. After that, the default handler comes back, so a second Ctrl-C kills the process.
- **Process groups:** each yt-dlp runs in its own group (`Setpgid`). On cancel, `Cmd.Cancel` sends SIGTERM to the whole group (yt-dlp and its ffmpeg). If they are still alive after `WaitDelay` (3 s), they get SIGKILL.
- **Pending and running jobs** end as `canceled`, and the program exits with code 130.
