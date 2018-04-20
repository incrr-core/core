package generate

import (
	"fmt"
	"go/ast"
	"sort"
	"strings"
)

func (bu *build) interfaceSignature(name string, it *ast.InterfaceType, i int) string {
	var methods []string
	for _, f := range it.Methods.List {
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}

			var sig string
			switch is := f.Type.(type) {
			case *ast.FuncType:
				sig = bu.funcTypeSignature(is, false) // this is from an interface, so we're not returning... // unicode.IsUpper(rune(name[0]))
			default:
				start1, end1 := bu.pos(f.Type.Pos()), bu.pos(f.Type.End())
				str(bu.file, start1, end1)
			}

			start, end := bu.pos(n.Pos()), bu.pos(n.End())
			methods = append(methods, fmt.Sprintf("%s %s", str(bu.file, start, end), sig))
		}

	}
	sort.Strings(methods)
	methodList := strings.Join(methods, ", ")
	if len(methodList) > 0 {
		methodList = " " + methodList + " "
	}
	return fmt.Sprintf("%s interface {%s}", name, methodList)
}

func (bu *build) interfaceJSON(name string, it *ast.InterfaceType) map[string]map[string][]string {
	var rtn = make(map[string]map[string][]string)

	for _, f := range it.Methods.List {
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}

			var methodFunc = make(map[string][]string)
			switch is := f.Type.(type) {
			case *ast.FuncType:
				methodFunc = bu.funcJSON(is)
			}

			start, end := bu.pos(n.Pos()), bu.pos(n.End())
			mname := str(bu.file, start, end)
			rtn[mname] = methodFunc
		}

	}
	return rtn
}
