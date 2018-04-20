package generate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func doBuildFileIntern(buildFilenameIntern string) (rtn struct {
	file            *os.File
	buildExtHash    string
	buildExtHashMap map[string]signature
}, err error) {
	rtn.buildExtHashMap = make(map[string]signature)

	rtn.file, err = os.OpenFile(buildFilenameIntern, os.O_RDWR, 0777)
	if err == nil {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, buildFilenameIntern, nil, 0)
		log.OnErr(err).Fatalf("fileset: %v", err)

		for _, d := range node.Decls {
			if gen, ok := d.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if val, ok := spec.(*ast.ValueSpec); ok {
						for _, id := range val.Names {
							switch id.Name {
							case "extHash":
								for _, v := range val.Values {
									rtn.buildExtHash = str(rtn.file, int64(v.Pos()), int64(v.End())-2)
								}
							case "extHashMap":
								for _, vv := range val.Values {
									for _, ve := range vv.(*ast.CompositeLit).Elts {
										k := ve.(*ast.KeyValueExpr).Key.(*ast.BasicLit)
										v := ve.(*ast.KeyValueExpr).Value.(*ast.CompositeLit).Elts
										v0 := v[0].(*ast.KeyValueExpr).Value.(*ast.BasicLit)
										v1 := v[1].(*ast.KeyValueExpr).Value.(*ast.BasicLit)

										key := str(rtn.file, int64(k.Pos()), int64(k.End())-2)
										rtn.buildExtHashMap[key] = signature{
											Sig: str(rtn.file, int64(v0.Pos()), int64(v0.End())-2),
											Dig: str(rtn.file, int64(v1.Pos()), int64(v1.End())-2),
										}
									}
								}
							}
						}
					}
				}
			}
		}
	} else if _, err := os.Stat(buildFilenameIntern); os.IsNotExist(err) {
		rtn.file, err = os.Create(buildFilenameIntern)
		log.OnErr(err).Fatalf("create: %v", err)
	} else {
		log.Fatalf("bad file: %s: %v", buildFilenameIntern, err)
	}
	return rtn, err
}
