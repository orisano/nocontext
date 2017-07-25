package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/pkg/errors"
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

func main() {
	var fname string
	flag.StringVar(&fname, "f", os.Getenv("GOFILE"), "parsing file")
	flag.Parse()

	f, err := getAST(fname)
	if err != nil {
		log.Fatal(fmt.Errorf("getAST failed %s", err))
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

		rname := fn.Recv.List[0].Names[0].Name
		r := ""
		switch recvType := fn.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			r = fmt.Sprintf("(%s *%s)", rname, recvType.X)
		}

		fmt.Printf("func %s %s(", r, fn.Name)

		args := []string{}
		for _, field := range fn.Type.Params.List[1:] {
			arg := []string{}
			for _, name := range field.Names {
				arg = append(arg, name.Name)
			}
			t := ""
			switch argType := field.Type.(type) {
			case *ast.SelectorExpr:
				t = fmt.Sprintf("%s.%s", argType.X, argType.Sel)
			case *ast.Ident:
				t = fmt.Sprintf("%s", argType.Name)
			}
			args = append(args, strings.Join(arg, ",")+" "+t)
		}
		fmt.Printf("%s)\n", strings.Join(args, ", "))
	}
}
