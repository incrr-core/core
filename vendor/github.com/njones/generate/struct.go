package generate

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"
)

func (bu *build) structSignature(name string, st *ast.StructType, i int) string {
	var fields []string
	for _, f := range st.Fields.List {
		switch nt := f.Type.(type) {
		case *ast.StructType:
			s := bu.structSignature(f.Names[0].Name, nt, i+1)
			fields = append(fields, s)
			continue
		}
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}
			start, end := bu.pos(n.Pos()), bu.pos(n.End())
			start1, end1 := bu.pos(f.Type.Pos()), bu.pos(f.Type.End())
			fields = append(fields, fmt.Sprintf("%s %s", str(bu.file, start, end), str(bu.file, start1, end1)))
		}
	}
	sort.Strings(fields)
	fieldList := strings.Join(fields, ", ")
	if len(fieldList) > 0 {
		fieldList = " " + fieldList + " "
	}
	return fmt.Sprintf("%s struct {%s}", name, fieldList)
}

func (bu *build) structJSON(st *ast.StructType) map[string]interface{} {
	rtn := map[string]interface{}{}

	for _, f := range st.Fields.List {
		switch nt := f.Type.(type) {
		case *ast.StructType:
			rtn[f.Names[0].Name] = bu.structJSON(nt)
			continue
		}
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}
			startk, endk := bu.pos(n.Pos()), bu.pos(n.End())
			startv, endv := bu.pos(f.Type.Pos()), bu.pos(f.Type.End())
			rtn[str(bu.file, startk, endk)] = str(bu.file, startv, endv)
		}
	}
	return rtn
}
