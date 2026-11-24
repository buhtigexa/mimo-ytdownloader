// Package architecture enforces the hexagonal dependency rule with go/parser:
//
//	infrastructure -> application -> domain
//
// domain and application must not import outer layers or I/O-heavy packages.
package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDependenciesPointInward(t *testing.T) {
	rules := []struct {
		layer     string
		forbidden []string // import path prefixes
	}{
		{"domain", []string{"github.com/buhtigexa/mimo-ytdownloader/internal/application", "github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure", "os", "os/exec", "net", "go.yaml.in"}},
		{"application", []string{"github.com/buhtigexa/mimo-ytdownloader/internal/infrastructure", "os", "os/exec", "net", "go.yaml.in"}},
	}
	fset := token.NewFileSet()
	for _, r := range rules {
		root := filepath.Join("..", r.layer)
		files := 0
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			files++
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				for _, bad := range r.forbidden {
					if p == bad || strings.HasPrefix(p, bad+"/") {
						t.Errorf("%s: %s must not import %q", path, r.layer, p)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if files == 0 {
			t.Errorf("layer %s: no files found", r.layer)
		}
	}
}
