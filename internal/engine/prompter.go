package engine

import "context"

// Prompter is an engine that can run a system + user prompt of the caller's own and return the
// model's answer (issue #48: the retranslation in a context the user gave). Only the LLM engines
// are one. System translation (the Translation framework) takes a text and a language pair and
// nothing else, so it is not: a context cannot reach it.
type Prompter interface {
	Prompt(ctx context.Context, system, user string) (string, error)
}
