package main

import (
	"fmt"
	"regexp"
	"strings"
)

// partialRE matches {{> name key=value …}} tokens that may appear in
// markdown content. They expand BEFORE goldmark sees the body so the
// output flows through standard markdown rendering.
var partialRE = regexp.MustCompile(`\{\{>\s*([a-z_][a-z0-9_]*)((?:\s+[a-z_][a-z0-9_]*="[^"]*")*)\s*\}\}`)

// argRE pulls out each name="value" pair inside a partial invocation.
var argRE = regexp.MustCompile(`([a-z_][a-z0-9_]*)="([^"]*)"`)

// partials is the registry of known partial names → handler. Handlers
// take the parsed args map and return the markdown (or raw HTML) that
// should replace the token. Returning an error aborts the build.
//
// v1 ships empty; placeholders for future expansion live below.
var partials = map[string]func(map[string]string) (string, error){
	// Example wired so the codepath is exercised; remove once a real
	// partial lands.
	"hello": func(args map[string]string) (string, error) {
		return "Hello " + args["name"] + "!", nil
	},
}

// expandPartials replaces every {{> name …}} token in body with the
// output of its handler. Unknown partials produce a build error.
func expandPartials(body string) (string, error) {
	var firstErr error
	out := partialRE.ReplaceAllStringFunc(body, func(match string) string {
		groups := partialRE.FindStringSubmatch(match)
		name := groups[1]
		args := parsePartialArgs(groups[2])
		handler, ok := partials[name]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("unknown partial %q", name)
			}
			return match
		}
		expanded, err := handler(args)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("partial %q: %w", name, err)
			}
			return match
		}
		return expanded
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func parsePartialArgs(s string) map[string]string {
	args := map[string]string{}
	for _, m := range argRE.FindAllStringSubmatch(strings.TrimSpace(s), -1) {
		args[m[1]] = m[2]
	}
	return args
}
