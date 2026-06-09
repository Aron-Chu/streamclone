package tokenize

import (
	"strings"
	"sync/atomic"

	"streamclone/internal/chat/batch"
)

type Emote struct {
	ID       string
	URL      string
	Zw       bool
	Provider string
}

type node struct {
	children map[rune]*node
	emote    *Emote
}

type Trie struct {
	root *node
}

func NewTrie() *Trie {
	return &Trie{root: &node{}}
}

func (t *Trie) Insert(name string, e Emote) {
	n := t.root
	for _, r := range name {
		if n.children == nil {
			n.children = make(map[rune]*node)
		}
		next, ok := n.children[r]
		if !ok {
			next = &node{}
			n.children[r] = next
		}
		n = next
	}
	cp := e
	n.emote = &cp
}

func (t *Trie) match(word string) (*Emote, bool) {
	n := t.root
	for _, r := range word {
		next, ok := n.children[r]
		if !ok {
			return nil, false
		}
		n = next
	}
	if n.emote == nil {
		return nil, false
	}
	return n.emote, true
}

func (t *Trie) splitZeroWidthSuffix(word string) (base string, overlays []matchedEmote, ok bool) {
	runes := []rune(word)
	for baseLen := len(runes) - 1; baseLen > 0; baseLen-- {
		candidate := string(runes[:baseLen])
		if _, matched := t.match(candidate); !matched {
			continue
		}
		suffix := string(runes[baseLen:])
		parts, matched := t.matchZeroWidthChain(suffix)
		if matched {
			return candidate, parts, true
		}
	}
	return "", nil, false
}

type matchedEmote struct {
	name  string
	emote *Emote
}

func (t *Trie) matchZeroWidthChain(text string) ([]matchedEmote, bool) {
	var out []matchedEmote
	remaining := []rune(text)
	for len(remaining) > 0 {
		bestLen := 0
		var best *Emote
		for i := 1; i <= len(remaining); i++ {
			name := string(remaining[:i])
			emote, ok := t.match(name)
			if ok && emote.Zw {
				bestLen = i
				best = emote
			}
		}
		if bestLen == 0 {
			return nil, false
		}
		out = append(out, matchedEmote{name: string(remaining[:bestLen]), emote: best})
		remaining = remaining[bestLen:]
	}
	return out, len(out) > 0
}

func (t *Trie) Tokenize(text string) []batch.Fragment {
	if len(text) == 0 {
		return []batch.Fragment{{T: "text", C: ""}}
	}

	var frags []batch.Fragment
	pending := ""

	flush := func() {
		if pending != "" {
			frags = append(frags, batch.Fragment{T: "text", C: pending})
			pending = ""
		}
	}

	i := 0
	runes := []rune(text)
	n := len(runes)

	for i < n {
		if runes[i] == ' ' {
			j := i
			for j < n && runes[j] == ' ' {
				j++
			}
			pending += string(runes[i:j])
			i = j
			continue
		}

		j := i
		for j < n && runes[j] != ' ' {
			j++
		}
		word := string(runes[i:j])

		if e, ok := t.match(word); ok {
			if e.Zw && len(frags) > 0 && frags[len(frags)-1].T == "emote" && strings.TrimSpace(pending) == "" {
				pending = ""
			}
			flush()
			frags = append(frags, fragmentForEmote(word, e))
		} else if base, overlays, ok := t.splitZeroWidthSuffix(word); ok {
			baseEmote, _ := t.match(base)
			flush()
			frags = append(frags, fragmentForEmote(base, baseEmote))
			for _, overlay := range overlays {
				frags = append(frags, fragmentForEmote(overlay.name, overlay.emote))
			}
		} else if mention, suffix, ok := splitMentionToken(word); ok {
			flush()
			frags = append(frags, batch.Fragment{T: "mention", C: mention})
			pending += suffix
		} else {
			pending += word
		}
		i = j
	}

	flush()
	return frags
}

func fragmentForEmote(name string, e *Emote) batch.Fragment {
	if e == nil {
		return batch.Fragment{T: "text", C: name}
	}
	return batch.Fragment{T: "emote", C: name, U: e.URL, Zw: e.Zw, ID: e.ID, Provider: e.Provider}
}

type ChannelDict struct {
	ptr atomic.Pointer[Trie]
}

func (d *ChannelDict) Swap(t *Trie) {
	d.ptr.Store(t)
}

func (d *ChannelDict) Tokenize(text string) []batch.Fragment {
	t := d.ptr.Load()
	if t == nil {
		return []batch.Fragment{{T: "text", C: text}}
	}
	return t.Tokenize(text)
}

func splitMentionToken(word string) (mention string, suffix string, ok bool) {
	if len(word) < 2 || word[0] != '@' {
		return "", "", false
	}
	end := 1
	for end < len(word) {
		ch := word[end]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			end++
			continue
		}
		break
	}
	if end == 1 {
		return "", "", false
	}
	return word[:end], word[end:], true
}
