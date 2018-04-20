// Package generate is for helping automatically generate new version
// numbers for software development.
//
// It's recommended that the `build.go` and `build_internal.go` files be
// added to the repo by developers. Each developer should keep their own
// files locally. The the committed build.go and build_internal.go files
// would come from an official build server. However the build number
// should be shared by all developers. As this will serve as a relative
// clock that can give general build information when placed in contex.
//
// The author is using `./git/info/exclude` to prevent the files from being
// uploaded to the git repo
package generate

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"hash"
	"io"
	"os"
	"sort"
	"strconv"
	"time"
	"unicode"

	"github.com/njones/logger"
)

var log = logger.New()

type bumpType int

const (
	bumpPatch bumpType = iota
	bumpMinor
	bumpMajor
)

// visitorFunc is the ast function for each node
type visitorFunc func(n ast.Node) ast.Visitor

// Visit satisfies the Visitor interface
func (f visitorFunc) Visit(n ast.Node) ast.Visitor {
	return f(n)
}

// BuildContext holds all of the build data
type BuildContext struct {
	BuildDir,
	BuildFilename,
	BuildFilenameIntern,
	BuildArtifact string
	BuildHashFunc func() (string, error)
}

type signature struct {
	Kind, JSON, Recv, Sig, Dig string
}

type build struct {
	file  *os.File
	files map[string]*ast.File

	name   string
	offset int64
	hash   hash.Hash

	structNames    map[string]signature
	funcNames      map[string]signature
	interfaceNames map[string]signature
	returnTypes    map[string]struct{} // keeps a list of return types so we can see exported items on them or not
}

func newBuild() *build {
	bu := new(build)
	bu.hash = sha1.New()
	bu.funcNames = make(map[string]signature)
	bu.structNames = make(map[string]signature)
	bu.interfaceNames = make(map[string]signature)
	bu.returnTypes = make(map[string]struct{})
	return bu
}

// pos returns the text postion int64 with
// the file offset included
func (bu *build) pos(t token.Pos) int64 {
	return int64(t) - bu.offset
}

// findTypes walks the node
func (bu *build) findTypes(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.Package:
		bu.files = n.Files
		return visitorFunc(bu.findTypes)
	case *ast.File:
		bu.offset = int64(n.Pos())
		if bu.file != nil {
			bu.file.Close()
			bu.file = nil
		}
		for k, v := range bu.files {
			if v == n {
				var err error
				bu.file, err = os.Open(k)
				log.OnErr(err).Fatalf("opening file: %v", err)
				return visitorFunc(bu.findTypes)
			}
		}
		return visitorFunc(bu.findTypes)
	case *ast.GenDecl:
		if n.Tok == token.TYPE {
			return visitorFunc(bu.findTypes)
		}
	case *ast.FuncDecl:
		rec, sig := bu.funcSignature(n)
		j := bu.funcJSON(n.Type)
		b, err := json.Marshal(&j)
		log.OnErr(err).Fatalf("marshal func type: %v", err)

		bu.funcNames[n.Name.Name] = signature{
			Kind: "func",
			JSON: string(b),
			Recv: rec,
			Sig:  sig,
			Dig:  hashToStr(bu.hash, sig),
		}
	case *ast.StructType:
		sig := bu.structSignature(bu.name, n, 0)
		j := bu.structJSON(n)
		b, err := json.Marshal(&j)
		log.OnErr(err).Fatalf("marshal struct type: %v", err)

		bu.structNames[bu.name] = signature{
			Kind: "struct",
			JSON: string(b),
			Sig:  sig,
			Dig:  hashToStr(bu.hash, sig),
		}
	case *ast.InterfaceType:
		sig := bu.interfaceSignature(bu.name, n, 0)
		j := bu.interfaceJSON(bu.name, n)
		b, err := json.Marshal(&j)
		log.OnErr(err).Fatalf("marshal interface type: %v", err)

		bu.interfaceNames[bu.name] = signature{
			Kind: "interface",
			JSON: string(b),
			Sig:  sig,
			Dig:  hashToStr(bu.hash, sig),
		}
	case *ast.TypeSpec:
		bu.name = n.Name.Name
		return visitorFunc(bu.findTypes)
	}
	return nil
}

func str(f io.ReaderAt, start, end int64) string {
	b := make([]byte, end-start)
	f.ReadAt(b, start)
	return string(b)
}

func MainBuild(ctx BuildContext) {

	var (
		buildDir            = ctx.BuildDir
		buildFilename       = ctx.BuildFilename
		buildFilenameIntern = ctx.BuildFilenameIntern
	)

	var buildNum int
	var buildSemVer = "0.0.0"
	var buildExtHash string
	var buildExtSame bool

	fs := token.NewFileSet()
	pkgs, err := parser.ParseDir(fs, buildDir, nil, 0)
	log.OnErr(err).Fatalf("parse packages: %v", err)

	bu := newBuild()
	for _, pkg := range pkgs {
		ast.Walk(visitorFunc(bu.findTypes), pkg)
	}

	bf, err := doBuildFile(buildFilename)
	log.OnErr(err).Fatalf("build file: %v", err)
	defer bf.file.Close()

	bfi, err := doBuildFileIntern(buildFilenameIntern)
	log.OnErr(err).Fatalf("build file: %v", err)
	defer bfi.file.Close()

	buildNum = bf.buildNum
	buildSemVer = bf.buildSemVer

	buildNum++

	newExtMap := make(map[string]signature)
	buKeys := make([]string, 0)
	for _, allSignatures := range []map[string]signature{
		bu.funcNames,
		bu.structNames,
		bu.interfaceNames,
	} {
		for k, v := range allSignatures {
			if unicode.IsUpper(rune(k[0])) {
				if len(v.Recv) > 0 {
					_, recvOk := bu.returnTypes[v.Recv]
					if unicode.IsLower(rune(v.Recv[0])) {
						if !recvOk {
							continue
						}
					}
				}
				newExtMap[k] = v
				buKeys = append(buKeys, k)
			} else if _, ok := bu.returnTypes[k]; ok {
				newExtMap[k] = v
				buKeys = append(buKeys, k)
			}
		}
	}

	sort.Strings(buKeys)

	// find the hash after sorting
	bu.hash.Reset()
	for _, v := range buKeys {
		bu.hash.Write([]byte(newExtMap[v].Dig))
	}

	bumpMode := bumpPatch
	buildExtHash, buildExtSame = checkExtHash(buildExtHash, hex.EncodeToString(bu.hash.Sum(nil)))
	if !buildExtSame {
		bumpMode = bumpToMode(buildSemVer, bfi.buildExtHashMap, newExtMap)
	}

	buildSemVer, err = bump(buildSemVer, bumpMode)
	log.OnErr(err).Fatalf("bump: %v", err)

	gitHash, err := ctx.BuildHashFunc()
	log.OnErr(err).Fatalf("git: %v", err)

	var pkgData = struct {
		BldDate, BldHash,
		BldNumber, BldSemVer string
		ExtHash      string
		ExtHashSlice []string
		ExtHashMap   map[string]signature
		Timestamp    string
	}{
		Timestamp:    time.Now().Format(time.RFC3339Nano),
		ExtHash:      buildExtHash,
		ExtHashSlice: buKeys,
		ExtHashMap:   newExtMap,

		BldDate:   time.Now().Format(time.RFC1123),
		BldHash:   string(gitHash[:8]),
		BldNumber: strconv.Itoa(buildNum),
		BldSemVer: buildSemVer,
	}

	bf.file.Truncate(0)
	err = buildTmpl.Execute(bf.file, pkgData)
	if err != nil {
		log.Fatalf("template: %v", err)
	}

	bfi.file.Truncate(0)
	err = buildInternTmpl.Execute(bfi.file, pkgData)
	if err != nil {
		log.Fatalf("template: %v", err)
	}
}
