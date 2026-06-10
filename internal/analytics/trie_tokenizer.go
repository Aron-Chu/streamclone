package analytics

import (
	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/tokenize"
)

type TrieTokenizer struct {
	Trie *tokenize.Trie
}

func (t *TrieTokenizer) Tokenize(_ string, text string) []batch.Fragment {
	if t == nil || t.Trie == nil {
		return []batch.Fragment{{T: "text", C: text}}
	}
	return t.Trie.Tokenize(text)
}
