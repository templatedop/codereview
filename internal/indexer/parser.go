package indexer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// CodeElement represents a parsed code element (function, type, etc.)
type CodeElement struct {
	Type        string   `json:"type"`        // "function", "method", "struct", "interface", "const", "var"
	Name        string   `json:"name"`        // Element name
	Package     string   `json:"package"`     // Package name
	FilePath    string   `json:"file_path"`   // Relative file path
	Line        int      `json:"line"`        // Line number
	Signature   string   `json:"signature"`   // Function signature or type definition
	Doc         string   `json:"doc"`         // Documentation comment
	Body        string   `json:"body"`        // Full source code
	Receiver    string   `json:"receiver"`    // Method receiver (if method)
	Imports     []string `json:"imports"`     // Imports used
	References  []string `json:"references"`  // Types/functions referenced
}

// Parser parses Go source files
type Parser struct {
	fset *token.FileSet
}

// NewParser creates a new Go parser
func NewParser() *Parser {
	return &Parser{
		fset: token.NewFileSet(),
	}
}

// ParseDirectory parses all Go files in a directory recursively
func (p *Parser) ParseDirectory(rootDir string) ([]CodeElement, error) {
	var elements []CodeElement

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip vendor, testdata, hidden dirs
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only parse .go files, skip tests
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileElements, err := p.ParseFile(path, rootDir)
		if err != nil {
			fmt.Printf("Warning: failed to parse %s: %v\n", path, err)
			return nil
		}

		elements = append(elements, fileElements...)
		return nil
	})

	return elements, err
}

// ParseFile parses a single Go file
func (p *Parser) ParseFile(filePath, rootDir string) ([]CodeElement, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	file, err := parser.ParseFile(p.fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(rootDir, filePath)
	if relPath == "" {
		relPath = filePath
	}

	var elements []CodeElement
	pkgName := file.Name.Name

	// Extract imports
	var imports []string
	for _, imp := range file.Imports {
		imports = append(imports, strings.Trim(imp.Path.Value, `"`))
	}

	// Process declarations
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			elem := p.parseFunction(d, src, pkgName, relPath, imports)
			elements = append(elements, elem)

		case *ast.GenDecl:
			elems := p.parseGenDecl(d, src, pkgName, relPath, imports)
			elements = append(elements, elems...)
		}
	}

	return elements, nil
}

// parseFunction extracts function/method information
func (p *Parser) parseFunction(fn *ast.FuncDecl, src []byte, pkg, filePath string, imports []string) CodeElement {
	elem := CodeElement{
		Type:     "function",
		Name:     fn.Name.Name,
		Package:  pkg,
		FilePath: filePath,
		Line:     p.fset.Position(fn.Pos()).Line,
		Imports:  imports,
	}

	// Check if it's a method
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		elem.Type = "method"
		elem.Receiver = p.nodeToString(fn.Recv.List[0].Type, src)
	}

	// Get signature
	elem.Signature = p.buildFuncSignature(fn, src)

	// Get documentation
	if fn.Doc != nil {
		elem.Doc = fn.Doc.Text()
	}

	// Get full body
	start := p.fset.Position(fn.Pos()).Offset
	end := p.fset.Position(fn.End()).Offset
	if end <= len(src) {
		elem.Body = string(src[start:end])
	}

	return elem
}

// parseGenDecl extracts type, const, var declarations
func (p *Parser) parseGenDecl(decl *ast.GenDecl, src []byte, pkg, filePath string, imports []string) []CodeElement {
	var elements []CodeElement

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			elem := CodeElement{
				Name:     s.Name.Name,
				Package:  pkg,
				FilePath: filePath,
				Line:     p.fset.Position(s.Pos()).Line,
				Imports:  imports,
			}

			switch s.Type.(type) {
			case *ast.StructType:
				elem.Type = "struct"
			case *ast.InterfaceType:
				elem.Type = "interface"
			default:
				elem.Type = "type"
			}

			if decl.Doc != nil {
				elem.Doc = decl.Doc.Text()
			}

			start := p.fset.Position(s.Pos()).Offset
			end := p.fset.Position(s.End()).Offset
			if end <= len(src) {
				elem.Body = string(src[start:end])
			}

			// Build signature
			elem.Signature = fmt.Sprintf("type %s %s", s.Name.Name, p.getTypeKind(s.Type))

			elements = append(elements, elem)

		case *ast.ValueSpec:
			for _, name := range s.Names {
				elem := CodeElement{
					Name:     name.Name,
					Package:  pkg,
					FilePath: filePath,
					Line:     p.fset.Position(name.Pos()).Line,
					Imports:  imports,
				}

				switch decl.Tok {
				case token.CONST:
					elem.Type = "const"
				case token.VAR:
					elem.Type = "var"
				}

				if decl.Doc != nil {
					elem.Doc = decl.Doc.Text()
				}

				start := p.fset.Position(s.Pos()).Offset
				end := p.fset.Position(s.End()).Offset
				if end <= len(src) {
					elem.Body = string(src[start:end])
				}

				elements = append(elements, elem)
			}
		}
	}

	return elements
}

// buildFuncSignature creates a readable function signature
func (p *Parser) buildFuncSignature(fn *ast.FuncDecl, src []byte) string {
	var sb strings.Builder

	sb.WriteString("func ")

	// Receiver
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sb.WriteString("(")
		sb.WriteString(p.nodeToString(fn.Recv.List[0].Type, src))
		sb.WriteString(") ")
	}

	sb.WriteString(fn.Name.Name)
	sb.WriteString("(")

	// Parameters
	if fn.Type.Params != nil {
		params := []string{}
		for _, param := range fn.Type.Params.List {
			paramType := p.nodeToString(param.Type, src)
			if len(param.Names) > 0 {
				for _, name := range param.Names {
					params = append(params, name.Name+" "+paramType)
				}
			} else {
				params = append(params, paramType)
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}
	sb.WriteString(")")

	// Return type
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		sb.WriteString(" ")
		if len(fn.Type.Results.List) > 1 {
			sb.WriteString("(")
		}
		results := []string{}
		for _, result := range fn.Type.Results.List {
			results = append(results, p.nodeToString(result.Type, src))
		}
		sb.WriteString(strings.Join(results, ", "))
		if len(fn.Type.Results.List) > 1 {
			sb.WriteString(")")
		}
	}

	return sb.String()
}

func (p *Parser) nodeToString(node ast.Node, src []byte) string {
	start := p.fset.Position(node.Pos()).Offset
	end := p.fset.Position(node.End()).Offset
	if start >= 0 && end <= len(src) {
		return string(src[start:end])
	}
	return ""
}

func (p *Parser) getTypeKind(t ast.Expr) string {
	switch t.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.ArrayType:
		return "array"
	case *ast.MapType:
		return "map"
	case *ast.ChanType:
		return "chan"
	default:
		return "alias"
	}
}
