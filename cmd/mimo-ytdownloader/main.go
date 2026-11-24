// Command mimo-ytdownloader downloads the audio of every URL in a file, in parallel,
// via yt-dlp.
//
// main is the composition root: it loads config, builds the adapters, wires
// them to the use case through its ports, and hands control to the CLI
// controller. It holds no business logic.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"syscall"

	portout "github.com/buhtigexa/mimo-ytdownloader/internal/application/port/out"
	"github.com/buhtigexa/mimo-ytdownloader/internal/application/usecase"
	"github.com/buhtigexa/mimo-ytdownloader/internal/domain"
	"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure/adapter/in/cli"
	"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure/adapter/out/console"
	"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure/adapter/out/ytdlp"
	"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stderr))
}

// run is main without the process exit, so tests can call it.
func run(args []string, stdin io.Reader, stderr io.Writer) int {
	// Collect garbage at 50% heap growth instead of 100%: peak memory drops by
	// about a third and GC CPU stays negligible (this program mostly waits on
	// yt-dlp). An explicit GOGC in the environment wins.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}

	// The progress reporter and the Ctrl-C notice write from different
	// goroutines, so every write to stderr goes through one lock.
	stderr = &syncWriter{w: stderr}

	flags, err := cli.ParseFlags(args, stderr)
	if errors.Is(err, cli.ErrHelp) {
		return cli.ExitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "mimo-ytdownloader: %v\n\n%s", err, cli.Usage)
		return cli.ExitUsage
	}

	if flags.CPUProfile != "" {
		stop, err := startCPUProfile(flags.CPUProfile)
		if err != nil {
			fmt.Fprintf(stderr, "mimo-ytdownloader: %v\n", err)
			return cli.ExitUsage
		}
		defer stop()
	}

	// The first Ctrl-C (or SIGTERM) cancels ctx. stop() then restores the
	// default handler, so a second Ctrl-C kills the process right away.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	noticed := make(chan struct{})
	cancelNotice := context.AfterFunc(ctx, func() {
		defer close(noticed)
		stop()
		fmt.Fprintln(stderr, "\nmimo-ytdownloader: interrupted, stopping (Ctrl-C again to force)")
	})
	defer func() {
		// Normal exit: unregister the notice. Interrupted: wait until it is printed.
		if !cancelNotice() {
			<-noticed
		}
	}()

	sum, err := app(ctx, flags, stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "mimo-ytdownloader: %v\n", err)
		if errors.Is(err, context.Canceled) {
			return cli.ExitInterrupted
		}
		return cli.ExitUsage
	}
	return cli.ExitCode(sum)
}

// app wires everything and runs the batch.
func app(ctx context.Context, flags cli.Flags, stdin io.Reader, stderr io.Writer) (domain.Summary, error) {
	cfg, err := config.Load(flags.Config)
	if err != nil {
		return domain.Summary{}, err
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return domain.Summary{}, err
	}

	// out adapters
	props := cfg.YtDlpProperties()
	downloader, err := ytdlp.New(props)
	if err != nil {
		return domain.Summary{}, err
	}
	version, err := downloader.Preflight(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	searchFailed := 0
	if flags.URLColumn == "url" {
		searchFailed, err = cli.EnrichCSV(ctx, flags.Input, ytdlp.NewSongResolver(props.Bin), stderr, cfg.DownloadPolicy())
		if err != nil {
			return domain.Summary{}, err
		}
	}
	reporter := console.New(stderr, flags.Verbose)
	reporter.SetSearchFailures(searchFailed)
	var playlists portout.PlaylistResolver // nil interface: expansion disabled
	if cfg.ExpandPlaylists {
		playlists = ytdlp.NewResolver(props)
	}

	// use case
	uc := usecase.NewDownloadBatch(downloader, playlists, reporter, cfg.DownloadPolicy())

	// in adapter
	fmt.Fprintf(stderr, "mimo-ytdownloader: %s -> %s as %s (up to %d workers, yt-dlp %s, %d already in archive)\n\n",
		flags.Input, cfg.OutputDir, props.Format, cfg.Workers, version, downloader.Archived())
	sum, err := cli.NewController(uc, stdin, flags.URLColumn).Handle(ctx, flags.Input)
	sum.Failed += searchFailed
	sum.Total += searchFailed
	return sum, err
}

func startCPUProfile(path string) (stop func(), err error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}, nil
}

// syncWriter makes an io.Writer safe for concurrent use.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
