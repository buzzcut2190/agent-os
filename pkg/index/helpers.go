package index

import (
	"go/ast"
	"strings"
)

// kindFromType maps an AST type expression to a SymbolKind.
func kindFromType(expr ast.Expr) SymbolKind {
	if expr == nil {
		return KindType
	}
	switch expr.(type) {
	case *ast.InterfaceType:
		return KindInterface
	case *ast.StructType:
		return KindClass
	default:
		return KindType
	}
}

// typeSignature produces a concise string representation of an AST type expression.
func typeSignature(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeSignature(t.X)
	case *ast.SelectorExpr:
		return typeSignature(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeSignature(t.Elt)
		}
		return "[...]" + typeSignature(t.Elt)
	case *ast.MapType:
		return "map[" + typeSignature(t.Key) + "]" + typeSignature(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + typeSignature(t.Value)
		case ast.RECV:
			return "<-chan " + typeSignature(t.Value)
		default:
			return "chan " + typeSignature(t.Value)
		}
	case *ast.FuncType:
		return funcSignature(t)
	case *ast.InterfaceType:
		return "interface{...}"
	case *ast.StructType:
		return "struct{...}"
	case *ast.Ellipsis:
		return "..." + typeSignature(t.Elt)
	default:
		return ""
	}
}

// funcSignature returns a string like "func(a int, b string) error".
func funcSignature(ft *ast.FuncType) string {
	var b strings.Builder
	b.WriteString("func(")
	if ft.Params != nil {
		for i, p := range ft.Params.List {
			if i > 0 {
				b.WriteString(", ")
			}
			for j, name := range p.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(name.Name)
			}
			if len(p.Names) > 0 {
				b.WriteString(" ")
			}
			b.WriteString(typeSignature(p.Type))
		}
	}
	b.WriteString(")")
	if ft.Results != nil && len(ft.Results.List) > 0 {
		b.WriteString(" ")
		if len(ft.Results.List) > 1 {
			b.WriteString("(")
		}
		for i, r := range ft.Results.List {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(typeSignature(r.Type))
		}
		if len(ft.Results.List) > 1 {
			b.WriteString(")")
		}
	}
	return b.String()
}

// calleeName tries to extract the name of the function being called.
func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return typeSignature(fn.X) + "." + fn.Sel.Name
	default:
		return ""
	}
}

// isStdlib returns true if the import path looks like a Go standard library package.
func isStdlib(path string) bool {
	return !strings.Contains(path, ".")
}
