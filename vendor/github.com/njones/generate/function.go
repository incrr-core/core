package generate

import (
	"fmt"
	"go/ast"
	"strings"
)

func (bu *build) funcSignature(fn *ast.FuncDecl) (string, string) {
	var rtnRecv, recv string
	if fn.Recv != nil {
		var vl []string
		for _, d := range fn.Recv.List {
			start, end := bu.pos(d.Type.Pos()), bu.pos(d.Type.End())
			recvType := fmt.Sprintf("%s", str(bu.file, start, end))
			vl = append(vl, recvType)
		}
		recv = strings.Join(vl, ", ")
		rtnRecv = strings.Trim(recv, "*")
		if len(recv) > 0 {
			recv = " (" + recv + ")"
		}
	}

	funcType := bu.funcTypeSignature(fn.Type, fn.Name.IsExported())
	return rtnRecv, strings.TrimSpace(fmt.Sprintf("func%s %s%s", recv, fn.Name.Name, funcType))
}

func (bu *build) funcTypeSignature(fn *ast.FuncType, isExported bool) string {
	var params string
	if fn.Params != nil {
		var pl []string
		for _, d := range fn.Params.List {
			start, end := bu.pos(d.Type.Pos()), bu.pos(d.Type.End())
			pl = append(pl, fmt.Sprintf("%s", str(bu.file, start, end)))
		}
		params = strings.Join(pl, ", ")
	}

	var results string
	if fn.Results != nil {
		var rl []string
		for _, d := range fn.Results.List {
			start, end := bu.pos(d.Type.Pos()), bu.pos(d.Type.End())
			rtnType := fmt.Sprintf("%s", fmt.Sprintf("%s", str(bu.file, start, end)))
			rl = append(rl, rtnType)
			if isExported {
				bu.returnTypes[strings.TrimLeft(rtnType, "*")] = struct{}{}
			}
		}

		results = strings.Join(rl, ", ")
		if len(rl) > 1 {
			results = fmt.Sprintf("(%s)", strings.Join(rl, ", "))
		}
	}

	return strings.TrimSpace(fmt.Sprintf("(%s) %s", params, results))
}

func (bu *build) funcJSON(fn *ast.FuncType) map[string][]string {
	rtn := map[string][]string{
		"params":  []string{},
		"results": []string{},
	}
	if fn.Params != nil {
		for _, d := range fn.Params.List {
			start, end := bu.pos(d.Type.Pos()), bu.pos(d.Type.End())
			rtn["params"] = append(rtn["params"], str(bu.file, start, end))
		}
	}

	if fn.Results != nil {
		for _, d := range fn.Results.List {
			start, end := bu.pos(d.Type.Pos()), bu.pos(d.Type.End())
			rtn["results"] = append(rtn["results"], str(bu.file, start, end))
		}
	}
	return rtn
}
