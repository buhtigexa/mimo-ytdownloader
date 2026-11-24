// Command ytgrab downloads the audio of every URL in a file, in parallel,
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
	// Collect garbage at 50% heap growth instead of 100%: peak memory drops by
	// about a third and GC CPU stays negligible (this program mostly waits on
	// yt-dlp). An explicit GOGC in the environment wins.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stderr))
}

func run(args []string, stdin io.Reader, stderr io.Writer) int {
	flags, err := cli.ParseFlags(args, stderr)
	if errors.Is(err, cli.ErrHelp) {
		return cli.ExitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "ytgrab: %v\n\n%s", err, cli.Usage)
		return cli.ExitUsage
	}

	if flags.CPUProfile != "" {
		stop, err := startCPUProfile(flags.CPUProfile)
		if err != nil {
			fmt.Fprintf(stderr, "ytgrab: %v\n", err)
			return cli.ExitUsage
		}
		defer stop()
	}

	// First SIGINT/SIGTERM cancels ctx; once ctx is done, stop() restores the
	// default handler so a second Ctrl-C kills the process right away.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			stop()
			fmt.Fprintln(stderr, "\nytgrab: interrupted, stopping (Ctrl-C again to force)")
		case <-done:
		}
	}()

	sum, err := app(ctx, flags, stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "ytgrab: %v\n", err)
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
	reporter := console.New(stderr, flags.Verbose)
	var playlists portout.PlaylistResolver // nil interface: expansion disabled
	if cfg.ExpandPlaylists {
		playlists = ytdlp.NewResolver(props)
	}

	// use case
	uc := usecase.NewDownloadBatch(downloader, playlists, reporter, cfg.DownloadPolicy())

	// in adapter
	fmt.Fprintf(stderr, "ytgrab: %s -> %s as %s (up to %d workers, yt-dlp %s, %d already in archive)\n\n",
		flags.Input, cfg.OutputDir, props.Format, cfg.Workers, version, downloader.Archived())
	return cli.NewController(uc, stdin, flags.URLColumn).Handle(ctx, flags.Input)
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
