package ytdlp

import (
	"bufio"
	"errors"
	"hash/maphash"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
)

// archive mirrors yt-dlp's --download-archive file ("youtube <id>" per line)
// in memory, so already-downloaded videos are skipped without spawning a
// yt-dlp process at all. yt-dlp itself keeps appending to the file; we read
// it once at startup and then record what we save.
//
// Without it, a re-run over a list of a million URLs would start a million
// Python processes just to learn they are already done.
//
// Memory: ids loaded at startup live in a sorted []uint64 of 64-bit hashes,
// 8 bytes each (8M archived videos = 64 MB), looked up by binary search.
// Videos saved during this run go to a small map.
type archive struct {
	seed   maphash.Seed
	loaded []uint64 // sorted, read-only after load: no lock needed

	mu    sync.RWMutex
	added map[uint64]struct{}
}

// loadArchive reads path. A missing file is an empty archive.
func loadArchive(path string) (*archive, error) {
	a := &archive{seed: maphash.MakeSeed(), added: make(map[uint64]struct{})}
	if path == "" {
		return a, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if fi, err := f.Stat(); err == nil {
		a.loaded = make([]uint64, 0, fi.Size()/20) // ~20 bytes per "youtube <id>\n"
	}
	sc := bufio.NewScanner(bufio.NewReaderSize(f, readBuf))
	for sc.Scan() {
		extractor, id, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if ok && strings.EqualFold(extractor, "youtube") && id != "" {
			a.loaded = append(a.loaded, maphash.String(a.seed, id))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	slices.Sort(a.loaded)
	a.loaded = slices.Compact(a.loaded)
	a.loaded = slices.Clip(a.loaded)
	return a, nil
}

func (a *archive) has(id string) bool {
	k := maphash.String(a.seed, id)
	if _, ok := slices.BinarySearch(a.loaded, k); ok {
		return true
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.added[k]
	return ok
}

func (a *archive) add(id string) {
	k := maphash.String(a.seed, id)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.added[k] = struct{}{}
}

func (a *archive) len() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.loaded) + len(a.added)
}

// readBuf is the read buffer for the archive file.
const readBuf = 1 << 20
