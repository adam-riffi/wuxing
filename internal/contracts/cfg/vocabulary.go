package cfg

// Vocabulary is the set of tool operations a workflow step may call. The cfg
// grammar is derived from it: Validate rejects any step naming a tool.operation
// not present here. It is supplied (not hardcoded into Validate) so it can grow
// with the tool catalogs in docs/vocabulary/.
type Vocabulary map[string]map[string]bool

// Known reports whether tool.operation is in the vocabulary.
func (v Vocabulary) Known(tool, operation string) bool {
	ops, ok := v[tool]
	if !ok {
		return false
	}
	return ops[operation]
}

// DefaultVocabulary is the operation set a service workflow may call today,
// mirroring docs/vocabulary/. Library/graph/processors are not service-callable
// workflow verbs, so they are absent.
func DefaultVocabulary() Vocabulary {
	return Vocabulary{
		"connectors": {"read": true, "query": true, "write": true, "upsert": true},
		"ai":         {"infer": true, "autocomplete": true, "agent": true},
	}
}
