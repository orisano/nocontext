package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func parseFile(path string) (*ast.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}
	defer f.Close()
	return parser.ParseFile(token.NewFileSet(), path, f, 0)
}

func run() error {
	fileName := flag.String("f", os.Getenv("GOFILE"), "target file")
	dirName := flag.String("d", "", "target directory")
	outputName := flag.String("o", "", "output filename")

	flag.Parse()

	if *fileName == "" && *dirName == "" {
		flag.Usage()
		return errors.New("require -f or -d")
	}
	if *fileName != "" && *dirName != "" {
		flag.Usage()
		return errors.New("either -f or -d, not both")
	}

	var fileNames []string
	switch {
	case *fileName != "":
		fileNames = append(fileNames, *fileName)
	case *dirName != "":
		infoList, err := ioutil.ReadDir(*dirName)
		if err != nil {
			return errors.Wrap(err, "failed to read dir")
		}
		for _, info := range infoList {
			name := info.Name()
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			fileNames = append(fileNames, filepath.Join(*dirName, name))
		}
	}

	var w io.Writer = os.Stdout
	if *outputName != "" {
		f, err := os.Create(*outputName)
		if err != nil {
			return errors.Wrap(err, "failed to create file")
		}
		defer f.Close()
		w = f
	}

	for _, fpath := range fileNames {
		if fpath == *outputName {
			continue
		}
		f, err := parseFile(fpath)
		if err != nil {
			log.Print("failed to parse:", err)
			continue
		}
		for _, decl := range f.Decls {
			fdecl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !fdecl.Name.IsExported() {
				continue
			}
			if !strings.HasSuffix(fdecl.Name.Name, "WithContext") {
				continue
			}

			name := fdecl.Name.Name
			fdecl.Name.Name = strings.TrimSuffix(fdecl.Name.Name, "WithContext")
			fdecl.Type.Params.List = fdecl.Type.Params.List[1:]

			var fun ast.Expr
			if len(fdecl.Recv.List) > 0 {
				fun = &ast.SelectorExpr{X: ast.NewIdent(fdecl.Recv.List[0].Names[0].Name), Sel: ast.NewIdent(name)}
			} else {
				fun = ast.NewIdent(name)
			}

			callExpr := &ast.CallExpr{
				Fun: fun,
				Args: []ast.Expr{
					&ast.CallExpr{
						Fun:  &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Background")},
						Args: []ast.Expr{},
					},
				},
			}

			for _, param := range fdecl.Type.Params.List {
				for _, name := range param.Names {
					callExpr.Args = append(callExpr.Args, name)
				}
			}

			if len(fdecl.Type.Results.List) > 0 {
				fdecl.Body.List = []ast.Stmt{
					&ast.ReturnStmt{
						Results: []ast.Expr{callExpr},
					},
				}
			} else {
				fdecl.Body.List = []ast.Stmt{
					&ast.ExprStmt{
						X: callExpr,
					},
				}
			}
			printer.Fprint(w, token.NewFileSet(), fdecl)
			fmt.Fprintln(w)
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
