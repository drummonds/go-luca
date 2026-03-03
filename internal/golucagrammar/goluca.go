// Package golucagrammar provides the embedded tree-sitter grammar for .goluca files.
package golucagrammar

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

//go:embed grammar_blobs/goluca.bin
var grammarBlobFS embed.FS

var (
	langOnce sync.Once
	lang     *gotreesitter.Language
	langErr  error
)

// GolucaLanguage returns the goluca language definition.
func GolucaLanguage() *gotreesitter.Language {
	langOnce.Do(func() {
		data, err := grammarBlobFS.ReadFile("grammar_blobs/goluca.bin")
		if err != nil {
			langErr = fmt.Errorf("read grammar blob: %w", err)
			return
		}
		gzr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			langErr = fmt.Errorf("open gzip: %w", err)
			return
		}
		defer gzr.Close()
		var l gotreesitter.Language
		if err := gob.NewDecoder(gzr).Decode(&l); err != nil {
			langErr = fmt.Errorf("decode grammar: %w", err)
			return
		}
		lang = &l
	})
	if langErr != nil {
		panic(fmt.Sprintf("golucagrammar: %v", langErr))
	}
	return lang
}
