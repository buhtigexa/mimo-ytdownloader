# mimo-ytdownloader

Download MP3 audio from a URL, a text file with one URL per line, or a CSV. For a CSV with `artist` and `title` columns, the app searches for missing URLs and saves them in the same file.

This project is intended for learning and experimentation. Its [0BSD license](LICENSE) covers the source code, not the media downloaded with it. Download only content you are permitted to obtain under applicable law and the platform's terms.

## Docker

From the repository root, build the image:

```bash
docker compose build mimo-ytdownloader
```

Set the directory containing your files, then run the app with a direct URL, a URL list, or a CSV:

```bash
export MIMO_YTDOWNLOADER_DATA_DIR="/path/to/your/files"
docker compose run --rm mimo-ytdownloader "https://www.youtube.com/watch?v=VIDEO_ID"
docker compose run --rm mimo-ytdownloader /data/urls.txt
docker compose run --rm mimo-ytdownloader /data/songs.csv
```

`MIMO_YTDOWNLOADER_DATA_DIR` is the host directory containing your URL lists and CSV files. The container sees it as `/data`. If you omit the variable, Compose uses `./data` in the repository root.

Downloaded files go to the `out/` subdirectory of the selected host directory. Found URLs are saved to the original CSV after the search finishes. Progress and errors appear in the terminal.

Each run starts up to 4 concurrent downloads by default; the configurable maximum is 5. By default, each download transfers one fragment at a time.
