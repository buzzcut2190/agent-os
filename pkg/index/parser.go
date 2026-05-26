package index

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// discoverGoFiles walks root and returns all .go file paths, skipping vendor
// and testdata directories.
func discoverGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// extractSymbols traverses a Go AST and returns every Symbol found.
//
// Fully qualified names follow the conventions:
//
//	function   → pkgDir.FuncName
//	method     → pkgDir.(*TypeName).Method
//	type       → pkgDir.TypeName
//	variable   → pkgDir.VarName
//	constant   → pkgDir.ConstName
func fqn(pkgDir, name string) string {
	if pkgDir == "." {
		return "." + name
	}
	return pkgDir + "." + name
}

func extractSymbols(fname, pkgDir, pkgName string, fset *token.FileSet, file *ast.File) []Symbol {
	var symbols []Symbol

	// Package-level declarations (GenDecl) cover types, constants, variables, imports.
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, spec := range gen.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if s.Name == nil {
					continue
				}
				kind := kindFromType(s.Type)
				pos := fset.Position(s.Pos())
				symbols = append(symbols, Symbol{
					Name: fqn(pkgDir, s.Name.Name),
					Kind:      kind,
					Package:   pkgName,
					Signature: typeSignature(s.Type),
					Def:       Position{File: fname, Line: pos.Line, Column: pos.Column},
				})

			case *ast.ValueSpec:
				for _, ident := range s.Names {
					kind := KindVariable
					if gen.Tok == token.CONST {
						kind = KindConstant
					}
					pos := fset.Position(ident.Pos())
					typeStr := ""
					if s.Type != nil {
						typeStr = typeSignature(s.Type)
					}
					symbols = append(symbols, Symbol{
						Name: fqn(pkgDir, ident.Name),
						Kind:      kind,
						Package:   pkgName,
						Signature: typeStr,
						Def:       Position{File: fname, Line: pos.Line, Column: pos.Column},
					})
				}
			}
		}
	}

	// Function declarations (FuncDecl) cover functions and methods.
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name == nil {
			continue
		}

		fnPos := fset.Position(fn.Pos())
		sig := funcSignature(fn.Type)

		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			// Method
			recvType := typeSignature(fn.Recv.List[0].Type)
			symbols = append(symbols, Symbol{
				Name: fqn(pkgDir, "("+recvType+")."+fn.Name.Name),
				Kind:      KindMethod,
				Package:   pkgName,
				Signature: fmt.Sprintf("(%s) %s", recvType, sig),
				Def:       Position{File: fname, Line: fnPos.Line, Column: fnPos.Column},
			})
		} else {
			// Function
			symbols = append(symbols, Symbol{
				Name: fqn(pkgDir, fn.Name.Name),
				Kind:      KindFunction,
				Package:   pkgName,
				Signature: sig,
				Def:       Position{File: fname, Line: fnPos.Line, Column: fnPos.Column},
			})
		}
	}

	return symbols
}

// extractImportsAndDeps extracts import statements from a Go file.
// Returns both the raw imports and any external dependencies.
func extractImportsAndDeps(fname, pkgDir, pkgName string, file *ast.File) ([]Dependency, []Dependency) {
	var imports, deps []Dependency

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == "" {
			continue
		}

		imports = append(imports, Dependency{
			From: pkgDir,
			To:   path,
			Kind: "import",
		})

		// External dependencies (non-stdlib) are tracked separately.
		if !isStdlib(path) {
			deps = append(deps, Dependency{
				From: pkgName,
				To:   path,
				Kind: "require",
			})
		}
	}

	return imports, deps
}

// extractCallEdges visits every CallExpr in the AST and records caller–callee relationships.
func extractCallEdges(fname, pkgDir, pkgName string, fset *token.FileSet, file *ast.File) []CallEdge {
	var edges []CallEdge

	var currentFunc string

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name != nil {
				if node.Recv != nil && len(node.Recv.List) > 0 {
					recvType := typeSignature(node.Recv.List[0].Type)
					currentFunc = fqn(pkgDir, "("+recvType+")."+node.Name.Name)
				} else {
					currentFunc = fqn(pkgDir, node.Name.Name)
				}
			}
		case *ast.CallExpr:
			if currentFunc == "" {
				return true
			}
			callee := calleeName(node)
			if callee == "" {
				return true
			}
			pos := fset.Position(node.Pos())
			edges = append(edges, CallEdge{
				Caller: currentFunc,
				Callee: callee,
				File:   fname,
				Line:   pos.Line,
			})
		}
		return true
	})

	return edges
}

