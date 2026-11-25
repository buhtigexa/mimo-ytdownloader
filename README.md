# mimo-ytdownloader

Download MP3 audio by default, or video with `--video`, by song title and artist, from a URL, from a text file with one URL per line, or from a CSV. For a CSV with `artist` and `title` columns, the app searches for missing URLs and saves them in the same file.

This project is intended for learning and experimentation. Its [0BSD license](LICENSE) covers the source code, not the media downloaded with it. Download only content you are permitted to obtain under applicable law and the platform's terms.

## Docker

From the repository root, build the image:

```bash
docker compose build mimo-ytdownloader
```

Set the directory containing your files, then run the app with a quoted song query, a direct URL, a URL list, or a CSV:

```bash
export MIMO_YTDOWNLOADER_DATA_DIR="/path/to/your/files"
docker compose run --rm mimo-ytdownloader "Mira a tu alrededor Alejandro Lerner"
docker compose run --rm mimo-ytdownloader "https://www.youtube.com/watch?v=VIDEO_ID"
docker compose run --rm mimo-ytdownloader --mp3 "https://www.youtube.com/watch?v=VIDEO_ID"
docker compose run --rm mimo-ytdownloader --video "https://www.youtube.com/watch?v=VIDEO_ID"
docker compose run --rm mimo-ytdownloader /data/urls.txt
docker compose run --rm mimo-ytdownloader /data/songs.csv
```

Docker Compose mounts `MIMO_YTDOWNLOADER_DATA_DIR` from your computer at `/data` inside the container. `/data` is a container path; you do not need to create a `data` folder inside your chosen directory.

| On your computer | Inside the container |
|---|---|
| `<chosen directory>/songs.csv` | `/data/songs.csv` |
| `<chosen directory>/out/` | `/data/out/` |

If you omit the variable, Compose uses `./data` in the repository root. Found URLs are saved to the original CSV after the search finishes. Progress and errors appear in the terminal.

Each successful download prints the full file path on your computer. With the directory above, files are stored under `/path/to/your/files/out/`.
For MP3 files, the completion line also shows the title and artist read from the file's ID3 tags when available; otherwise it keeps the input song label or URL. Tags reflect the source file and may be inaccurate.
Skipped downloads also show the existing file's ID3 tags and full path when its filename contains the video ID. If an archive entry has no matching file under `out/`, the CLI reports that its path is unavailable.

`--mp3` is optional: the Docker image uses MP3 by default. `--video` downloads video with audio, preferring MP4 when available. Audio and video use separate download archives, so saving one does not skip the other.

For a song query, include the title and artist in one quoted argument. The app searches YouTube, checks candidate recordings and available audio quality, and reports a failed search if it cannot verify a suitable match. MP3 files are saved in the `out/` directory shown above.

Each run starts up to 4 concurrent downloads by default; the configurable maximum is 5. By default, each download transfers one fragment at a time.
