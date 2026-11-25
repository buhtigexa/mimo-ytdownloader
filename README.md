# mimo-ytdownloader

Download MP3 audio by default, or video with `--video`. Use a YouTube URL, a song title and artist, a URL list, or a CSV.

## Build

Run from the repository directory:

```bash
docker compose build mimo-ytdownloader
```

For a complete rebuild, use `docker compose build --no-cache --pull mimo-ytdownloader`.

## Files and output

```bash
export MIMO_YTDOWNLOADER_DATA_DIR="/absolute/path/to/your/music"
```

Compose mounts that directory at `/data` **inside the container**:

- Input: `<host directory>/songs.csv` → `/data/songs.csv` (same for `urls.txt`).
- Output: `/data/out/` → `<host directory>/out/` on your computer. The CLI prints full host paths.

If the variable is unset, Compose uses `./data` in the repository.

## Run

```bash
docker compose run --rm mimo-ytdownloader "https://www.youtube.com/watch?v=VIDEO_ID"      # MP3
docker compose run --rm mimo-ytdownloader --video "https://www.youtube.com/watch?v=VIDEO_ID" # video with audio
docker compose run --rm mimo-ytdownloader "Mira a tu alrededor Alejandro Lerner"         # search by song
docker compose run --rm mimo-ytdownloader /data/urls.txt                                  # one URL per line
docker compose run --rm mimo-ytdownloader /data/songs.csv                                 # artist,title CSV
```

`--mp3` is optional. Repeated URLs are skipped; when the existing file can be found, the CLI shows its full path and MP3 tags. For song CSVs, missing URLs are searched and saved back to the CSV.

This project is for learning and personal use. The [0BSD license](LICENSE) covers the code, not downloaded media. Download only content you are permitted to obtain.
