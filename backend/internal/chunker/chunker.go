package chunker

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

type Chunk struct {
	Index      int
	Content    string
	Hash       string
	TokenCount int
}

type Chunker struct {
	Size    int
	Overlap int
}

func CleanHTML(input string) string {
	root, err := xhtml.Parse(strings.NewReader(input))
	if err != nil {
		return normalizeSpace(stripTagsFallback(input))
	}
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript":
				return
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
				b.WriteString("\n")
			case "td", "th":
				b.WriteString(" | ")
			case "a":
				for _, a := range n.Attr {
					if a.Key == "href" && a.Val != "" {
						defer func(v string) { b.WriteString(" (" + v + ")") }(a.Val)
						break
					}
				}
			}
		}
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == xhtml.ElementNode {
			switch strings.ToLower(n.Data) {
			case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "table":
				b.WriteString("\n")
			}
		}
	}
	walk(root)
	return normalizeSpace(html.UnescapeString(b.String()))
}

func (c Chunker) Split(text string) []Chunk {
	text = normalizeSpace(text)
	if text == "" {
		return nil
	}
	size := c.Size
	if size <= 0 {
		size = 1600
	}
	overlap := c.Overlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size / 5
	}

	paras := regexp.MustCompile(`\n{2,}`).Split(text, -1)
	var chunks []Chunk
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s == "" {
			return
		}
		chunks = append(chunks, newChunk(len(chunks), s))
		cur.Reset()
		if overlap > 0 && len([]rune(s)) > overlap {
			r := []rune(s)
			cur.WriteString(string(r[len(r)-overlap:]))
			cur.WriteString(" ")
		}
	}
	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if cur.Len()+len(p)+2 > size {
			flush()
		}
		if len([]rune(p)) > size {
			r := []rune(p)
			for start := 0; start < len(r); {
				end := start + size
				if end > len(r) {
					end = len(r)
				}
				chunks = append(chunks, newChunk(len(chunks), string(r[start:end])))
				if end == len(r) {
					break
				}
				start = end - overlap
				if start < 0 {
					start = end
				}
			}
			cur.Reset()
			continue
		}
		cur.WriteString(p)
		cur.WriteString("\n\n")
	}
	flush()
	return chunks
}

func newChunk(i int, s string) Chunk {
	sum := sha256.Sum256([]byte(s))
	return Chunk{Index: i, Content: s, Hash: hex.EncodeToString(sum[:]), TokenCount: len(strings.Fields(s))}
}

func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func normalizeSpace(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []string
	re := regexp.MustCompile(`[ \t]+`)
	for _, line := range lines {
		line = strings.TrimSpace(re.ReplaceAllString(line, " "))
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n\n")
}

func stripTagsFallback(s string) string {
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, " ")
}
