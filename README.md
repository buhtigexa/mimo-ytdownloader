# mimo-ytdownloader

`ytgrab` downloads the audio from a list of URLs, in parallel, using [`yt-dlp`](https://github.com/yt-dlp/yt-dlp). You set it up with a YAML file.

> Only download content you have the right to download (your own, freely licensed, or with the owner's permission), and follow each site's terms of service.

## Requirements

Either **Docker** (nothing else needed), or:

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

## Run with Docker

The image has everything inside: `ytgrab`, `yt-dlp`, and `ffmpeg`. It runs as a non-root user.

| Path in the container | What it is |
|---|---|
| `/data` | Volume. Put your input files here. |
| `/data/out` | Downloads, plus the archive file that makes re-runs skip finished work. |
| `/etc/ytgrab/ytgrab.yml` | Config ([deploy/ytgrab.yml](deploy/ytgrab.yml)). Mount your own file here to change it. |

### Local, with Docker Compose

```bash
mkdir -p data
cp urls.txt data/                     # your list of URLs
docker compose run --rm ytgrab        # builds the image the first time
```

Files land in `./data/out`. To use another input file:

```bash
docker compose run --rm ytgrab /data/list.csv
```

On Linux, the container runs as user `1000:1000`, so the files in `./data` belong to you. If `id -u` shows another number, run `HOST_UID=$(id -u) HOST_GID=$(id -g) docker compose run --rm ytgrab`.

### Local, with plain Docker

```bash
make docker                           # same as: docker build -t mimo-ytdownloader .
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD/data:/data" mimo-ytdownloader
```

- **Use your own config:** add `-v "$PWD/my.yml:/etc/ytgrab/ytgrab.yml:ro"`.
- **Update yt-dlp** when YouTube changes break it: `docker build --build-arg YTDLP_VERSION=<version> -t mimo-ytdownloader .`
- **Stopping:** `docker stop` (or Ctrl-C) stops cleanly: running downloads are canceled and the exit code is `130`.

## Deploy to AWS

ytgrab is a batch job: it starts, works through the list, and exits. On AWS, run it as an **ECS Fargate task**, with an **EFS** file system mounted at `/data`, so the input files, the downloads, and the archive outlive each run.

**1. Push the image to ECR**

```bash
export AWS_REGION=us-east-1
export ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export REPO=$ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/mimo-ytdownloader

aws ecr create-repository --repository-name mimo-ytdownloader
aws ecr get-login-password | docker login --username AWS --password-stdin "${REPO%%/*}"
docker buildx build --platform linux/amd64 -t "$REPO:latest" --push .
```

**2. Create the task definition**

Fill in the `<...>` values in [deploy/ecs-task-definition.json](deploy/ecs-task-definition.json): account, region, EFS file system, and EFS access point. The access point should use user `1000` (UID and GID). Then register it:

```bash
aws ecs register-task-definition --cli-input-json file://deploy/ecs-task-definition.json
```

**3. Copy your URL list to EFS and run the task**

Put `urls.txt` at the root of the EFS access point (it shows up as `/data/urls.txt`). Then:

```bash
aws ecs run-task --cluster <CLUSTER> --launch-type FARGATE \
  --task-definition mimo-ytdownloader \
  --network-configuration "awsvpcConfiguration={subnets=[<SUBNET_ID>],securityGroups=[<SG_ID>],assignPublicIp=ENABLED}"
```

- **Logs** go to CloudWatch, in the `/ecs/mimo-ytdownloader` log group.
- **Another input file:** add `--overrides '{"containerOverrides":[{"name":"ytgrab","command":["/data/list.csv"]}]}'`.
- **Network:** the task needs outbound internet access, through a public IP (as above) or a NAT gateway.
- **Graviton (arm64):** build with `--platform linux/arm64` (or `linux/amd64,linux/arm64` for both) and set `"cpuArchitecture": "ARM64"` in the task definition. On an x86 machine, install ARM emulation once first: `docker run --privileged --rm tonistiigi/binfmt --install arm64`.
- **Rate limits:** sites often slow down requests from cloud IPs (HTTP 429). If you see many retries, lower `workers` in the config.
- **On a schedule:** create an EventBridge Scheduler rule that runs the same task.

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
