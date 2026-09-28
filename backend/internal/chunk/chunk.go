package chunk

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	StrategyAuto      = "auto"
	StrategyCustom    = "custom"
	StrategyHierarchy = "hierarchy"

	autoMax     = 800
	autoOverlap = 80
)

var (
	urlRe   = regexp.MustCompile(`https?://[^\s]+|[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}`)
	spaceRe = regexp.MustCompile(`[ \t\r\f\v]+`)
)

type Options struct {
	Strategy      string
	MaxLen        int
	OverlapRatio  float64
	StripURL      bool
	CollapseSpace bool
}

type Chunk struct {
	Index int
	Text  string
	Start int
	End   int
}

func NormalizeStrategy(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", StrategyAuto:
		return StrategyAuto
	case StrategyCustom:
		return StrategyCustom
	case StrategyHierarchy:
		return StrategyHierarchy
	default:
		return StrategyAuto
	}
}

// Split 把待切分文本切成切片。偏移相对于 src，不改写调用方保存的原文。
func Split(src string, opt Options) []Chunk {
	opt.Strategy = NormalizeStrategy(opt.Strategy)
	text := src
	if opt.Strategy == StrategyCustom {
		text = preprocess(src, opt)
	}
	var raw []Chunk
	switch opt.Strategy {
	case StrategyCustom:
		maxLen := opt.MaxLen
		if maxLen < 100 || maxLen > 2000 {
			maxLen = autoMax
		}
		overlap := int(float64(maxLen) * opt.OverlapRatio)
		if opt.OverlapRatio < 0 || opt.OverlapRatio > 0.5 {
			overlap = int(float64(maxLen) * 0.1)
		}
		raw = window(text, maxLen, overlap)
	case StrategyHierarchy:
		raw = hierarchy(text)
	default:
		raw = window(text, autoMax, autoOverlap)
	}
	for i := range raw {
		raw[i].Index = i
	}
	return raw
}

func preprocess(src string, opt Options) string {
	out := src
	if opt.StripURL {
		out = urlRe.ReplaceAllString(out, " ")
	}
	if opt.CollapseSpace {
		out = strings.ReplaceAll(out, "\n", " ")
		out = spaceRe.ReplaceAllString(out, " ")
		out = strings.TrimSpace(out)
	}
	return out
}

func window(text string, maxLen, overlap int) []Chunk {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if maxLen <= 0 {
		maxLen = autoMax
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxLen {
		overlap = maxLen / 10
	}
	var out []Chunk
	start := 0
	for start < len(runes) {
		end := start + maxLen
		if end >= len(runes) {
			out = append(out, Chunk{Text: string(runes[start:]), Start: start, End: len(runes)})
			break
		}
		cut := findBreak(runes[start:end])
		if cut <= 0 {
			cut = maxLen
		}
		end = start + cut
		out = append(out, Chunk{Text: string(runes[start:end]), Start: start, End: end})
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return out
}

func findBreak(window []rune) int {
	if len(window) == 0 {
		return 0
	}
	minKeep := len(window) / 2
	for i := len(window) - 1; i >= minKeep; i-- {
		if window[i] == '\n' && i > 0 && window[i-1] == '\n' {
			return i + 1
		}
	}
	for i := len(window) - 1; i >= minKeep; i-- {
		if window[i] == '\n' {
			return i + 1
		}
	}
	for i := len(window) - 1; i >= minKeep; i-- {
		if window[i] == '。' || window[i] == '！' || window[i] == '？' || window[i] == '.' || window[i] == '!' || window[i] == '?' {
			return i + 1
		}
	}
	return len(window)
}

func hierarchy(text string) []Chunk {
	lines := strings.Split(text, "\n")
	type section struct {
		text  string
		start int
	}
	var sections []section
	var buf []string
	secStart := 0
	offset := 0
	flush := func(end int) {
		if len(buf) == 0 {
			return
		}
		body := strings.Join(buf, "\n")
		if strings.TrimSpace(body) == "" {
			buf = nil
			return
		}
		sections = append(sections, section{text: body, start: secStart})
		buf = nil
		_ = end
	}
	for i, line := range lines {
		if isHeading(line) && len(buf) > 0 {
			flush(offset)
			secStart = offset
		}
		if len(buf) == 0 {
			secStart = offset
		}
		buf = append(buf, line)
		offset += len([]rune(line))
		if i < len(lines)-1 {
			offset++
		}
	}
	flush(offset)
	if len(sections) == 0 {
		return window(text, autoMax, autoOverlap)
	}
	var out []Chunk
	for _, sec := range sections {
		parts := window(sec.text, autoMax, autoOverlap)
		if len(parts) == 0 {
			continue
		}
		for _, p := range parts {
			p.Start += sec.start
			p.End += sec.start
			out = append(out, p)
		}
	}
	return out
}

func isHeading(line string) bool {
	s := strings.TrimLeftFunc(line, unicode.IsSpace)
	if !strings.HasPrefix(s, "#") {
		return false
	}
	n := 0
	for _, r := range s {
		if r == '#' {
			n++
			continue
		}
		return n >= 1 && n <= 3 && (r == ' ' || r == '\t')
	}
	return false
}
