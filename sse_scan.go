package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

type sseChoiceScan struct {
	index       int
	finish      string
	emptyFRSpan [2]int
}

type sseChunkScan struct {
	id, model string
	choices   []sseChoiceScan
}

// scanChunkData walks the JSON once without decoding content or constructing
// maps. Only escaped tracked strings need the JSON string decoder. Deliberate
// divergences: trailing bytes after the root and raw controls in skipped
// strings are accepted. Invalid structure and tracked types still fail closed.
func scanChunkData(data string) (sseChunkScan, bool) {
	p := chunkScanner{s: data}
	var out sseChunkScan
	ok := p.object(func(key string) bool {
		switch {
		case key == "id" || strings.EqualFold(key, "id"):
			return p.text(&out.id)
		case key == "model" || strings.EqualFold(key, "model"):
			return p.text(&out.model)
		case key == "choices" || strings.EqualFold(key, "choices"):
			out.choices = nil
			if p.null() {
				return true
			}
			return p.array(func() bool {
				ch := sseChoiceScan{emptyFRSpan: [2]int{-1, -1}}
				if !p.null() && !p.object(func(key string) bool {
					switch {
					case key == "index" || strings.EqualFold(key, "index"):
						if p.null() {
							return true
						}
						start := p.i
						if !p.number() {
							return false
						}
						n, err := strconv.Atoi(p.s[start:p.i])
						ch.index = n
						return err == nil
					case key == "finish_reason" || strings.EqualFold(key, "finish_reason"):
						ch.finish = ""
						ch.emptyFRSpan = [2]int{-1, -1}
						start := p.i
						if !p.text(&ch.finish) {
							return false
						}
						if p.s[start:p.i] == `""` {
							ch.emptyFRSpan = [2]int{start, p.i}
						}
						return true
					default:
						return p.value()
					}
				}) {
					return false
				}
				out.choices = append(out.choices, ch)
				return true
			})
		default:
			return p.value()
		}
	})
	return out, ok
}

type chunkScanner struct {
	s     string
	i     int
	depth int
}

func (p *chunkScanner) space() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

func (p *chunkScanner) take(c byte) bool {
	p.space()
	if p.i < len(p.s) && p.s[p.i] == c {
		p.i++
		return true
	}
	return false
}

func (p *chunkScanner) null() bool {
	p.space()
	if strings.HasPrefix(p.s[p.i:], "null") {
		p.i += 4
		return true
	}
	return false
}

// stringToken skips to the unescaped closing quote. For tracked values and
// object keys, also decode and validate the string exactly as encoding/json.
func (p *chunkScanner) stringToken(decode bool) (string, bool) {
	p.space()
	start := p.i
	if p.i >= len(p.s) || p.s[p.i] != '"' {
		return "", false
	}
	p.i++
	slow := false
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		switch c {
		case '\\':
			slow = true
			if p.i == len(p.s) {
				return "", false
			}
			p.i++
		case '"':
			if !decode {
				return "", true
			}
			raw := p.s[start+1 : p.i-1]
			if !slow && utf8.ValidString(raw) {
				return raw, true
			}
			var decoded string
			err := json.Unmarshal([]byte(p.s[start:p.i]), &decoded)
			return decoded, err == nil
		default:
			if c < 0x20 {
				slow = true
			}
		}
	}
	return "", false
}

func (p *chunkScanner) text(dst *string) bool {
	if p.null() {
		return true
	}
	s, ok := p.stringToken(true)
	if ok {
		*dst = s
	}
	return ok
}

func (p *chunkScanner) object(field func(string) bool) bool {
	if !p.take('{') || p.depth >= 10000 {
		return false
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.take('}') {
		return true
	}
	for {
		key, ok := p.stringToken(true)
		if !ok || !p.take(':') {
			return false
		}
		p.space()
		if !field(key) {
			return false
		}
		if p.take('}') {
			return true
		}
		if !p.take(',') {
			return false
		}
	}
}

func (p *chunkScanner) array(element func() bool) bool {
	if !p.take('[') || p.depth >= 10000 {
		return false
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.take(']') {
		return true
	}
	for {
		p.space()
		if !element() {
			return false
		}
		if p.take(']') {
			return true
		}
		if !p.take(',') {
			return false
		}
	}
}

func (p *chunkScanner) value() bool {
	p.space()
	if p.i == len(p.s) {
		return false
	}
	switch p.s[p.i] {
	case '{':
		return p.object(func(string) bool { return p.value() })
	case '[':
		return p.array(p.value)
	case '"':
		_, ok := p.stringToken(false)
		return ok
	case 'n':
		return p.null()
	case 't', 'f':
		for _, literal := range []string{"true", "false"} {
			if strings.HasPrefix(p.s[p.i:], literal) {
				p.i += len(literal)
				return true
			}
		}
		return false
	default:
		return p.number()
	}
}

func (p *chunkScanner) digit() bool {
	if p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
		return true
	}
	return false
}

func (p *chunkScanner) number() bool {
	p.space()
	if p.i < len(p.s) && p.s[p.i] == '-' {
		p.i++
	}
	if p.i < len(p.s) && p.s[p.i] == '0' {
		p.i++
	} else {
		if !p.digit() {
			return false
		}
		for p.digit() {
		}
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		if !p.digit() {
			return false
		}
		for p.digit() {
		}
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if !p.digit() {
			return false
		}
		for p.digit() {
		}
	}
	return true
}
