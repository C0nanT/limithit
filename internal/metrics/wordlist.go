package metrics

import (
	"bufio"
	_ "embed"
	"os"
	"strings"
	"sync/atomic"
)

//go:embed paths.txt
var defaultPaths string

type Wordlist struct {
	paths []string
	idx   atomic.Uint64
}

func DefaultWordlist() *Wordlist {
	return parseWordlist(defaultPaths)
}

func LoadWordlist(path string) (*Wordlist, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "/") {
			line = "/" + line
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &Wordlist{paths: lines}, nil
}

func parseWordlist(raw string) *Wordlist {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "/") {
			line = "/" + line
		}
		lines = append(lines, line)
	}
	return &Wordlist{paths: lines}
}

func (w *Wordlist) Next() string {
	if len(w.paths) == 0 {
		return "/"
	}
	n := w.idx.Add(1) - 1
	return w.paths[int(n%uint64(len(w.paths)))]
}

func (w *Wordlist) Size() int { return len(w.paths) }
func (w *Wordlist) All() []string {
	cp := make([]string, len(w.paths))
	copy(cp, w.paths)
	return cp
}
