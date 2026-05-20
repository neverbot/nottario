// Package main implements a tiny static analyser that flags pgx
// Query/Exec/QueryRow calls whose SQL argument is built unsafely
// INLINE at the call site.
//
// Rationale: gosec G201/G202 only know about database/sql sinks; with
// pgx we need a custom check. The check is intentionally narrow — we
// flag the OBVIOUS injection patterns at the call site:
//
//	pool.Exec(ctx, fmt.Sprintf("WHERE name = '%s'", req.Name))
//	pool.Exec(ctx, "DELETE FROM x WHERE id = '" + id + "'")
//
// We do NOT chase identifiers across the function. Code that builds
// queries into a local variable via fmt.Sprintf (the placeholder-
// index pattern we use everywhere: `query += fmt.Sprintf(" AND … =
// $%d", idx)`) is correct by construction in this repo today and
// will be eliminated by the sqlc migration (Tier 2 of the SQL safety
// feature). For deeper guarantees, lean on sqlc, not on this guard.
//
// Usage:
//
//	go run ./internal/tools/sqlcheck ./...
package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// pgx-side method names that accept SQL as their second arg (after ctx).
var sqlSinkMethods = map[string]bool{
	"Exec":     true,
	"Query":    true,
	"QueryRow": true,
}

func main() {
	patterns := os.Args[1:]
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedDeps,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(2)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(2)
	}
	violations := 0
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !sqlSinkMethods[sel.Sel.Name] {
					return true
				}
				if !isPgxReceiver(pkg.TypesInfo, sel.X) {
					return true
				}
				if len(call.Args) < 2 {
					return true
				}
				sqlArg := call.Args[1]
				if reason := danger(pkg.TypesInfo, sqlArg); reason != "" {
					pos := pkg.Fset.Position(sqlArg.Pos())
					fmt.Printf("%s:%d:%d: %s in inline argument to %s — use $N placeholders or move the query to sqlc\n",
						pos.Filename, pos.Line, pos.Column, reason, sel.Sel.Name)
					violations++
				}
				return true
			})
		}
	}
	if violations > 0 {
		fmt.Fprintf(os.Stderr, "sqlcheck: %d violation(s)\n", violations)
		os.Exit(1)
	}
}

// isPgxReceiver returns true when expr's type lives under the
// github.com/jackc/pgx hierarchy (pgxpool.Pool, pgx.Tx, pgx.Conn,
// pgxpool.Conn, etc.) so we don't fire on unrelated Query/Exec methods.
func isPgxReceiver(info *types.Info, expr ast.Expr) bool {
	if info == nil {
		return false
	}
	t := info.TypeOf(expr)
	if t == nil {
		return false
	}
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		// pgx.Tx is an interface; match by package path on its string.
		if t.String() != "" && strings.Contains(t.String(), "jackc/pgx") {
			return true
		}
		return false
	}
	if named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return strings.Contains(named.Obj().Pkg().Path(), "jackc/pgx")
}

// danger inspects the inline SQL expression at the call site and
// returns a short reason when the expression itself constructs SQL
// unsafely (Sprintf with a string-typed runtime value, or string
// concatenation with a runtime value). Identifier references that
// resolve to a compile-time constant are treated as literal.
func danger(info *types.Info, e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CallExpr:
		if isFmtSprintf(v) {
			for _, arg := range v.Args[1:] { // skip format string itself
				if isProbablyString(info, arg) {
					return "fmt.Sprintf interpolates a runtime string into SQL"
				}
			}
		}
		return ""
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			if !isLiteralOrConst(info, v) {
				return "string concatenation of a runtime value into SQL"
			}
		}
	}
	return ""
}

func isFmtSprintf(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "fmt" {
		return false
	}
	return sel.Sel.Name == "Sprintf"
}

// isProbablyString returns true when an expression looks like a
// runtime string-typed value worth flagging. Literals and compile-
// time constants (e.g. package-level `const channel = "x"`) are
// considered safe. Numeric / boolean values are also safe — the
// `$%d` placeholder-index pattern depends on that.
func isProbablyString(info *types.Info, e ast.Expr) bool {
	if _, ok := e.(*ast.BasicLit); ok {
		return false
	}
	if info != nil {
		// Compile-time constants are safe (their value is fixed at
		// build time, not influenced by runtime input).
		if tv, ok := info.Types[e]; ok && tv.Value != nil {
			return false
		}
		// Non-string types are safe (numeric placeholder indexes).
		if tv, ok := info.Types[e]; ok && tv.Type != nil {
			if b, isBasic := tv.Type.Underlying().(*types.Basic); isBasic {
				if b.Kind() != types.String && b.Kind() != types.UntypedString {
					return false
				}
			}
		}
	}
	return true
}

func isLiteralOrConst(info *types.Info, e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind == token.STRING
	case *ast.ParenExpr:
		return isLiteralOrConst(info, v.X)
	case *ast.BinaryExpr:
		return v.Op == token.ADD && isLiteralOrConst(info, v.X) && isLiteralOrConst(info, v.Y)
	case *ast.Ident, *ast.SelectorExpr:
		if info != nil {
			if tv, ok := info.Types[e]; ok && tv.Value != nil {
				return true
			}
		}
		return false
	}
	return false
}
