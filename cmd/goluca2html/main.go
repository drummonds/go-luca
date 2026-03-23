// goluca2html reads .goluca text from stdin and writes syntax-highlighted HTML to stdout.
// Output uses <span class="hl-{capture}"> tags suitable for embedding in <pre><code> blocks.
package main

import (
	"fmt"
	"html"
	"io"
	"os"

	"codeberg.org/hum3/gotreesitter"
	"codeberg.org/hum3/gotreesitter/grammars"
)

const highlightQuery = `
(date) @constant
(flag) @keyword
(payee) @string
(account) @type
(arrow) @operator
(linked_prefix) @operator
(description) @string
(amount) @number
(commodity) @constant
(comment) @comment
`

func main() {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading stdin: %v\n", err)
		os.Exit(1)
	}

	lang := grammars.GolucaLanguage()
	hl, err := gotreesitter.NewHighlighter(lang, highlightQuery)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating highlighter: %v\n", err)
		os.Exit(1)
	}

	ranges := hl.Highlight(src)

	var pos uint32
	for _, r := range ranges {
		// Emit unhighlighted text before this range.
		if r.StartByte > pos {
			fmt.Print(html.EscapeString(string(src[pos:r.StartByte])))
		}
		// Emit highlighted span.
		fmt.Printf(`<span class="hl-%s">%s</span>`, r.Capture, html.EscapeString(string(src[r.StartByte:r.EndByte])))
		pos = r.EndByte
	}
	// Emit trailing unhighlighted text.
	if int(pos) < len(src) {
		fmt.Print(html.EscapeString(string(src[pos:])))
	}
}
