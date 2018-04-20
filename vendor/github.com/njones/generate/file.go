package generate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
)

func doBuildFile(buildFilename string) (rtn struct {
	file        *os.File
	buildNum    int
	buildSemVer string
}, err error) {
	rtn.file, err = os.OpenFile(buildFilename, os.O_RDWR, 0777)
	if err == nil {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, buildFilename, nil, 0)
		log.OnErr(err).Fatalf("fileset: %v", err)

		for _, d := range node.Decls {
			if gen, ok := d.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if val, ok := spec.(*ast.ValueSpec); ok {
						for _, id := range val.Names {
							switch id.Name {
							case "verBuild":
								for _, v := range val.Values {
									i, err := strconv.Atoi(str(rtn.file, int64(v.Pos()), int64(v.End())-2))
									log.OnErr(err).Fatalf("check: %v", err)
									rtn.buildNum = i
								}
							case "verSemVer":
								for _, v := range val.Values {
									rtn.buildSemVer = str(rtn.file, int64(v.Pos()), int64(v.End())-2)
								}
							}
						}
					}
				}
			}
		}
	} else if _, err := os.Stat(buildFilename); os.IsNotExist(err) {
		rtn.file, err = os.Create(buildFilename)
		log.OnErr(err).Fatalf("create: %v", err)
	} else {
		log.Fatalf("bad file: %s: %v", buildFilename, err)
	}

	return rtn, err
}
