// Package cli is the driving adapter: it reads the command line and the URL
// list and calls the in.DownloadBatchUseCase input port.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const Usage = `usage: ytgrab [-c config.yml] [-v] [-url-column NAME] <urls.txt | urls.csv | ->

flags:
  -c, -config FILE      YAML config (default: ./ytgrab.yml)
  -v, -verbose          log when each worker picks up a URL
  -url-column NAME      CSV column holding the URLs (default: url)
  -cpuprofile FILE      write a CPU profile (for PGO: cp it to cmd/ytgrab/default.pgo)

input: *.csv -> CSV with a header row, URLs read from the url column;
       anything else -> one URL per line. Both are streamed (any size).

exit codes: 0 ok, 1 some downloads failed, 2 usage/config error, 130 interrupted
`

// ErrHelp is returned when -h / -help was requested.
var ErrHelp = flag.ErrHelp

// Flags is the parsed command line.
type Flags struct {
	Config     string
	Input      string
	Verbose    bool
	CPUProfile string
	URLColumn  string
}

// ParseFlags parses os.Args[1:] with the standard flag package.
func ParseFlags(args []string, stderr io.Writer) (Flags, error) {
	var f Flags
	fs := flag.NewFlagSet("ytgrab", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() { fmt.Fprint(stderr, Usage) }
	fs.StringVar(&f.Config, "c", "ytgrab.yml", "")
	fs.StringVar(&f.Config, "config", "ytgrab.yml", "")
	fs.BoolVar(&f.Verbose, "v", false, "")
	fs.BoolVar(&f.Verbose, "verbose", false, "")
	fs.StringVar(&f.CPUProfile, "cpuprofile", "", "")
	fs.StringVar(&f.URLColumn, "url-column", "url", "")

	// On -h/-help, fs.Parse itself calls fs.Usage and returns flag.ErrHelp.
	if err := fs.Parse(args); err != nil {
		return Flags{}, err
	}
	switch fs.NArg() {
	case 0:
		return Flags{}, errors.New("missing input file")
	case 1:
		f.Input = fs.Arg(0)
		return f, nil
	default:
		return Flags{}, errors.New("only one input file allowed")
	}
}
