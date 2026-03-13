// luca-docs renders markdown files to Bulma-styled HTML pages for the go-luca project.
// It handles regular docs, benchmark analysis (split-file merging), and schema docs (with mermaid support).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

const pageTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Title}} - go-luca</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@0.9.4/css/bulma.min.css">
    <style>
        pre {
            background-color: #2b2b2b;
            color: #f8f8f2;
            padding: 1.25em 1.5em;
            overflow-x: auto;
        }
        pre code {
            color: inherit;
            background: none;
            padding: 0;
            font-size: 0.875em;
        }
        code {
            background-color: #f5f5f5;
            color: #e83e8c;
            padding: 0.125em 0.375em;
            border-radius: 3px;
        }
        pre.mermaid {
            background-color: #ffffff;
            color: #333;
            text-align: center;
        }
    </style>
</head>
<body>
    <section class="section">
        <div class="container">
            <nav class="breadcrumb" aria-label="breadcrumbs">
                <ul>
                    <li><a href="{{.RootPath}}index.html">go-luca</a></li>
                    <li class="is-active"><a href="#" aria-current="page">{{.Title}}</a></li>
                </ul>
            </nav>
            <div class="content">
{{.Body}}
            </div>
        </div>
    </section>{{if .HasMermaid}}
    <script type="module">
        import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
        mermaid.initialize({ startOnLoad: true });
    </script>{{end}}
</body>
</html>
`

type pageData struct {
	Title      string
	RootPath   string
	Body       template.HTML
	HasMermaid bool
}

func main() {
	docsDir := flag.String("docs", "docs", "directory containing doc markdown files")
	benchDir := flag.String("benchmarks", "benchmarks", "directory containing benchmark subdirectories")
	researchDir := flag.String("research", "research", "directory containing research markdown files")
	schemaDir := flag.String("schema", "docs/schema", "directory containing schema markdown files")
	flag.Parse()

	tmpl := template.Must(template.New("page").Parse(pageTmpl))

	var failed bool

	// 1. Regular docs
	mds, _ := filepath.Glob(filepath.Join(*docsDir, "*.md"))
	for _, mdPath := range mds {
		outPath := strings.TrimSuffix(mdPath, ".md") + ".html"
		if err := convertFile(tmpl, mdPath, outPath, ""); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", mdPath, err)
			failed = true
			continue
		}
		fmt.Printf("%s -> %s\n", mdPath, outPath)
	}

	// 2. Benchmarks: each subdirectory of benchDir is a benchmark.
	//    If results.md exists, render it directly. Otherwise merge purpose + analysis + summary.
	entries, _ := os.ReadDir(*benchDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		topic := e.Name()
		subDir := filepath.Join(*benchDir, topic)

		// Prefer results.md (complete assembled report from bench run)
		var src []byte
		resultsPath := filepath.Join(subDir, "results.md")
		if data, err := os.ReadFile(resultsPath); err == nil {
			src = data
		} else {
			// Fallback: merge component files
			var md strings.Builder
			title := titleCase(strings.ReplaceAll(topic, "-", " "))
			md.WriteString("# " + title + " Benchmark\n\n")

			md.WriteString("## Purpose\n\n")
			appendFileContents(&md, filepath.Join(subDir, "purpose.md"))

			if data, err := os.ReadFile(filepath.Join(subDir, "analysis.md")); err == nil {
				md.WriteString("\n## Analysis\n\n")
				md.Write(data)
			}
			if data, err := os.ReadFile(filepath.Join(subDir, "summary.md")); err == nil {
				md.WriteString("\n## Summary\n\n")
				md.Write(data)
			}
			src = []byte(md.String())
		}

		outDir := filepath.Join(*docsDir, "benchmarks")
		os.MkdirAll(outDir, 0o755)
		outPath := filepath.Join(outDir, topic+".html")

		if err := renderMarkdown(tmpl, src, outPath, "../"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", topic, err)
			failed = true
			continue
		}
		fmt.Printf("%s -> %s\n", subDir, outPath)
	}

	// 3. Research: each .md file in researchDir is a standalone research document.
	researchMDs, _ := filepath.Glob(filepath.Join(*researchDir, "*.md"))
	if len(researchMDs) > 0 {
		outDir := filepath.Join(*docsDir, "research")
		os.MkdirAll(outDir, 0o755)
		for _, mdPath := range researchMDs {
			base := strings.TrimSuffix(filepath.Base(mdPath), ".md")
			outPath := filepath.Join(outDir, base+".html")
			if err := convertFile(tmpl, mdPath, outPath, "../"); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s: %v\n", mdPath, err)
				failed = true
				continue
			}
			fmt.Printf("%s -> %s\n", mdPath, outPath)
		}
	}

	// 4. Schema docs
	schemaMDs, _ := filepath.Glob(filepath.Join(*schemaDir, "*.md"))
	for _, mdPath := range schemaMDs {
		outPath := strings.TrimSuffix(mdPath, ".md") + ".html"
		if err := convertFile(tmpl, mdPath, outPath, "../"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", mdPath, err)
			failed = true
			continue
		}
		fmt.Printf("%s -> %s\n", mdPath, outPath)
	}

	if failed {
		os.Exit(1)
	}
}

func appendFileContents(sb *strings.Builder, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	sb.Write(data)
}

func convertFile(tmpl *template.Template, mdPath, outPath, rootPath string) error {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return err
	}
	return renderMarkdown(tmpl, data, outPath, rootPath)
}

func renderMarkdown(tmpl *template.Template, src []byte, outPath, rootPath string) error {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	// Extract title from first H1
	title := extractTitle(src)

	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return fmt.Errorf("goldmark: %w", err)
	}

	body := buf.String()

	// Rewrite .md links to .html (tbls generates .md inter-page links)
	body = strings.ReplaceAll(body, `.md"`, `.html"`)
	body = strings.ReplaceAll(body, `.md)`, `.html)`)

	// Handle mermaid code blocks: replace <pre><code class="language-mermaid">...</code></pre>
	// with <pre class="mermaid">...</pre>
	hasMermaid := strings.Contains(body, "language-mermaid")
	if hasMermaid {
		body = replaceMermaidBlocks(body)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, pageData{
		Title:      title,
		RootPath:   rootPath,
		Body:       template.HTML(body),
		HasMermaid: hasMermaid,
	})
}

func extractTitle(src []byte) string {
	reader := text.NewReader(src)
	p := parser.NewParser(parser.WithBlockParsers(parser.DefaultBlockParsers()...))
	doc := p.Parse(reader)

	var title string
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			var buf bytes.Buffer
			for c := h.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					buf.Write(t.Segment.Value(src))
				}
			}
			title = buf.String()
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})

	if title == "" {
		title = "Documentation"
	}
	return title
}

// replaceMermaidBlocks rewrites only mermaid code blocks, leaving other code blocks intact.
func replaceMermaidBlocks(body string) string {
	const open = `<pre><code class="language-mermaid">`
	const close = `</code></pre>`
	var out strings.Builder
	for {
		idx := strings.Index(body, open)
		if idx < 0 {
			out.WriteString(body)
			break
		}
		out.WriteString(body[:idx])
		body = body[idx+len(open):]
		// Find the matching </code></pre>
		end := strings.Index(body, close)
		if end < 0 {
			// Malformed — write the opening tag back and continue
			out.WriteString(open)
			out.WriteString(body)
			break
		}
		out.WriteString(`<pre class="mermaid">`)
		out.WriteString(body[:end])
		out.WriteString(`</pre>`)
		body = body[end+len(close):]
	}
	return out.String()
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
