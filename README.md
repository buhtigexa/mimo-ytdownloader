# mimo-ytdownloader

`ytgrab` downloads the audio from a list of URLs, in parallel, using [`yt-dlp`](https://github.com/yt-dlp/yt-dlp). You set it up with a YAML file.

> Only download content you have the right to download (your own, freely licensed, or with the owner's permission), and follow each site's terms of service.

## Requirements

- Go 1.27 or later
- `yt-dlp` (`pipx install yt-dlp`)
- `ffmpeg`, unless `audio.format` is `original`

## Install

```bash
git clone https://github.com/buhtigexa/mimo-ytdownloader.git
cd mimo-ytdownloader
make build
```

Or:

```bash
go install github.com/buhtigexa/mimo-ytdownloader/cmd/ytgrab@latest
```

## Usage

```bash
cp ytgrab.example.yml ytgrab.yml   # set output_dir and format
./bin/ytgrab urls.txt              # one URL per line
./bin/ytgrab list.csv              # CSV with a "url" column
./bin/ytgrab -                     # read from stdin
```

### Input

- **Text file:** one URL per line. Lines starting with `#` are ignored.
- **CSV file** (`*.csv`): needs a header row. URLs come from the `url` column, or the one you pick with `-url-column`. Quotes, multi-line fields, and BOM are supported.
- **Large files are fine:** input is streamed, never loaded all at once.
- **Bad rows** (not a URL, broken CSV) show up as `FAIL`, and the rest of the batch keeps going.
- **Search pages** (`youtube.com/results?...`) are rejected. The tool only downloads the URLs you give it.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | All OK |
| `1` | Some downloads failed |
| `2` | Usage or config error |
| `130` | Interrupted |

## How it works

- **Parallel:** runs several workers (`workers` setting), each with its own `yt-dlp` process.
- **Playlists:** a `youtube.com/playlist?list=...` link is expanded into its videos, and each one is downloaded in parallel. A single video inside a playlist (`watch?v=X&list=Y`) downloads only that video. Turn this off with `expand_playlists: false`.
- **Duplicates:** the same video under different URLs (`youtu.be/X`, `watch?v=X`, `/shorts/X`) is downloaded once.
- **Retries:** temporary errors (429, 5xx, timeouts) are retried. Permanent errors are not.
- **Safe to re-run:** finished downloads are saved in `.ytgrab-archive.txt` and skipped next time.
- **Ctrl-C:** stops cleanly and kills `yt-dlp` and `ffmpeg`. Press it twice to quit right away.

## Performance

Tested with a 10-million-row CSV (827 MB) where every video was already downloaded:

| | |
|---|---|
| Time | 31 s (~320,000 rows/s) |
| Peak memory | 556 MB |

- Memory grows with the number of *unique* videos (~50 bytes each), not with file size.
- Already-downloaded videos are checked in memory, without starting `yt-dlp`.
- If downloads are slow, reading slows down too, so memory stays flat.
- Static binary (~3 MB), built with `-trimpath -ldflags="-s -w"` and `CGO_ENABLED=0`.
- Built for modern CPUs by default (`GOAMD64=v3`). For older machines: `GOAMD64=v1 make build`.
- Profile-guided optimization: `make pgo URLS=urls.txt && make build`.

The real bottleneck is the network and `yt-dlp`, not this tool.

## Tests

```bash
make test    # vet + tests
make race    # with the race detector
make cover   # tests + coverage report, fails below 95%
make bench   # benchmarks
```

- Coverage is above 95% in every package.
- Tests use a fake `yt-dlp` shell script and a fake clock (`testing/synctest`), so they run fast and need no network. They run on Linux and macOS; the program itself also builds for Windows.
- End-to-end tests run the whole program, including Ctrl-C handling.
- An architecture test fails the build if `domain` or `application` import an outer layer.

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the code is organized.

## License

[0BSD](LICENSE): use, copy, change, and share it for any purpose, no conditions.
