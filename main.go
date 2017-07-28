package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/imports"
)

func getAST(filename string) (*ast.File, error) {
	src, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "read file failed: %s", filename)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, errors.Wrap(err, "parse file failed")
	}
	return f, nil
}

func getReceiver(decl *ast.FuncDecl) (string, string) {
	if len(decl.Recv.List) == 0 {
		return "", ""
	}
	name := decl.Recv.List[0].Names[0].Name
	typeStr := getType(decl.Recv.List[0].Type)
	return name, fmt.Sprintf("(%s %s)", name, typeStr)
}

func getType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", getType(t.X), t.Sel.Name)
	case *ast.StarExpr:
		return fmt.Sprintf("*%s", getType(t.X))
	case *ast.Ident:
		return t.Name
	case *ast.ArrayType:
		return fmt.Sprintf("[]%s", getType(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("[%s]%s", getType(t.Key), getType(t.Value))
	default:
		fmt.Printf("[DEBUG] expr = %#v\n", expr)
		return ""
	}
}

func getNames(fields []*ast.Field) []string {
	names := []string{}
	for _, field := range fields {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func getSignature(fields []*ast.Field) string {
	args := []string{}
	for _, field := range fields {
		arg := []string{}
		for _, name := range field.Names {
			arg = append(arg, name.Name)
		}
		argStr := strings.Join(arg, ", ")
		if len(argStr) > 0 {
			argStr += " "
		}
		args = append(args, fmt.Sprintf("%s%s", argStr, getType(field.Type)))
	}
	return strings.Join(args, ", ")
}

type GenSrc struct {
	body *bytes.Buffer
}

func NewGenSrc() *GenSrc {
	return &GenSrc{
		body: &bytes.Buffer{},
	}
}

func (g *GenSrc) Writer() io.Writer {
	return g.body
}

func (g *GenSrc) Generate() ([]byte, error) {
	return imports.Process("generated.go", g.body.Bytes(), nil)
}

func main() {
	defaultFile := os.Getenv("GOFILE")
	var fname string
	flag.StringVar(&fname, "f", defaultFile, "target file")
	flag.StringVar(&fname, "file", defaultFile, "target file")

	var dname string
	flag.StringVar(&dname, "d", "", "target directory")
	flag.StringVar(&dname, "dir", "", "target directory")

	var oname string
	flag.StringVar(&oname, "o", "", "output filename")
	flag.StringVar(&oname, "out", "", "output filename")

	flag.Parse()

	if len(fname) == 0 && len(dname) == 0 {
		flag.Usage()
		log.Fatal("require -f or -d")
	}
	if len(fname) > 0 && len(dname) > 0 {
		flag.Usage()
		log.Fatal("either -f or -d, not both")
	}

	filenames := make([]string, 0)
	if len(fname) > 0 {
		filenames = append(filenames, fname)
	} else {
		infos, err := ioutil.ReadDir(dname)
		if err != nil {
			log.Fatalf("read dir failed: %s", err)
		}
		for _, info := range infos {
			name := info.Name()
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			filenames = append(filenames, path.Join(dname, name))
		}
	}

	var oWriter io.Writer
	if len(oname) == 0 {
		oWriter = os.Stdout
	} else {
		f, err := os.Create(oname)
		if err != nil {
			log.Fatalf("create file failed: %s", err)
		}
		defer f.Close()
		oWriter = f
	}

	g := NewGenSrc()
	w := g.Writer()
	for i, filename := range filenames {
		if filename == oname {
			continue
		}
		f, err := getAST(filename)
		if err != nil {
			log.Fatalf("getAST failed %s", err)
		}

		if i == 0 {
			fmt.Fprintf(w, "package %s\n", f.Name.Name)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := fn.Name.String()
			if !('A' <= name[0] && name[0] <= 'Z') {
				continue
			}
			if !strings.HasSuffix(name, "WithContext") {
				continue
			}

			fnName := strings.TrimSuffix(name, "WithContext")
			recVar, recStr := getReceiver(fn)
			args := getSignature(fn.Type.Params.List[1:])
			results := getSignature(fn.Type.Results.List)
			names := getNames(fn.Type.Params.List[1:])
			names = append([]string{"context.Background()"}, names...)

			if len(recVar) > 0 {
				name = recVar + "." + name
			}
			if len(results) > 0 {
				results = "(" + results + ")"
			}
			fmt.Fprintf(w, "func %s %s(%s) %s {\r\n", recStr, fnName, args, results)
			fmt.Fprintf(w, "\treturn %s(%s)\r\n", name, strings.Join(names, ", "))
			fmt.Fprintln(w, "}")
		}
	}
	out, err := g.Generate()
	if err != nil {
		log.Fatalf("generate failed: %s", err)
	}
	if _, err := oWriter.Write(out); err != nil {
		log.Fatalf("write failed: %s", err)
	}
}
