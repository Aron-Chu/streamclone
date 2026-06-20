package matcher

// SemanticMatcher upgrades lexical matching with pgvector when PULSE_WIRE_SEMANTIC is on (Phase 4).
type SemanticMatcher struct {
	lexical *LexicalMatcher
	enabled bool
}

func NewSemantic(lexical *LexicalMatcher, enabled bool) *SemanticMatcher {
	return &SemanticMatcher{lexical: lexical, enabled: enabled}
}

// Score delegates to lexical until embedding columns exist.
func (m *SemanticMatcher) Score(in Input) Result {
	if m.lexical == nil {
		return Result{Decision: "discard"}
	}
	return m.lexical.Score(in)
}
