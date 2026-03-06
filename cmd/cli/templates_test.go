package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplatePathsExist(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		node, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}

		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != "templateFS" || sel.Sel.Name != "ReadFile" {
				return true
			}

			if len(call.Args) > 0 {
				arg := call.Args[0]

				if basicLit, ok := arg.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					templatePath := strings.Trim(basicLit.Value, `"`)
					_, err := templateFS.ReadFile(templatePath)
					if err != nil {
						t.Errorf("%s: templateFS.ReadFile(\"%s\") fails: %v", fset.Position(call.Pos()), templatePath, err)
					}
				}

				if fmtCall, ok := arg.(*ast.CallExpr); ok {
					if fmtSel, ok := fmtCall.Fun.(*ast.SelectorExpr); ok {
						if fmtIdent, ok := fmtSel.X.(*ast.Ident); ok && fmtIdent.Name == "fmt" && fmtSel.Sel.Name == "Sprintf" {
							if len(fmtCall.Args) > 0 {
								if formatLit, ok := fmtCall.Args[0].(*ast.BasicLit); ok && formatLit.Kind == token.STRING {
									formatStr := strings.Trim(formatLit.Value, `"`)
									if strings.Contains(formatStr, "auth_tables.%s.sql") {
										for _, db := range []string{"mysql", "postgres"} {
											path := strings.Replace(formatStr, "%s", db, 1)
											_, err := templateFS.ReadFile(path)
											if err != nil {
												t.Errorf("%s: templateFS.ReadFile(\"%s\") fails: %v", fset.Position(call.Pos()), path, err)
											}
										}
									}
								}
							}
						}
					}
				}
			}

			return true
		})
	}
}
