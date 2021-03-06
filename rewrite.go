package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"

	"github.com/tools/godep/Godeps/_workspace/src/github.com/kr/fs"
)

// rewrite visits the go files in pkgs, plus all go files
// in the directory tree Godeps, rewriting import statments
// according to the rules for func qualify.
func rewrite(pkgs []*Package, qual string, paths []string) error {
	for _, path := range pkgFiles(pkgs) {
		err := rewriteTree(path, qual, paths)
		if err != nil {
			return err
		}
	}
	return rewriteTree("Godeps", qual, paths)
}

// pkgFiles returns the full filesystem path to all go files in pkgs.
func pkgFiles(pkgs []*Package) []string {
	var a []string
	for _, pkg := range pkgs {
		for _, s := range pkg.allGoFiles() {
			a = append(a, filepath.Join(pkg.Dir, s))
		}
	}
	return a
}

// rewriteTree recursively visits the go files in path, rewriting
// import statments according to the rules for func qualify.
func rewriteTree(path, qual string, paths []string) error {
	w := fs.Walk(path)
	for w.Step() {
		if w.Err() != nil {
			log.Println("rewrite:", w.Err())
			continue
		}
		if !w.Stat().IsDir() && strings.HasSuffix(w.Path(), ".go") {
			err := rewriteGoFile(w.Path(), qual, paths)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// rewriteGoFile rewrites import statments in the named file
// according to the rules for func qualify.
func rewriteGoFile(name, qual string, paths []string) error {
	printerConfig := &printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 8}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	var changed bool
	for _, s := range f.Imports {
		name, err := strconv.Unquote(s.Path.Value)
		if err != nil {
			return err // can't happen
		}
		q := qualify(unqualify(name), qual, paths)
		if q != name {
			s.Path.Value = strconv.Quote(q)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	var buffer bytes.Buffer
	if err = printerConfig.Fprint(&buffer, fset, f); err != nil {
		return err
	}
	fset = token.NewFileSet()
	f, err = parser.ParseFile(fset, name, &buffer, parser.ParseComments)
	ast.SortImports(fset, f)
	wpath := name + ".temp"
	w, err := os.Create(wpath)
	if err != nil {
		return err
	}
	if err = printerConfig.Fprint(w, fset, f); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}

	return os.Rename(wpath, name)
}

// VendorExperiment is the Go 1.5 vendor directory experiment flag, see
// https://github.com/golang/go/commit/183cc0cd41f06f83cb7a2490a499e3f9101befff
var VendorExperiment = os.Getenv("GO15VENDOREXPERIMENT") == "1"

// sep is the signature set of path elements that
// precede the original path of an imported package.
var sep = defaultSep(VendorExperiment)

func defaultSep(experiment bool) string {
	if experiment {
		return "/vendor/"
	}
	return "/Godeps/_workspace/src/"
}

// unqualify returns the part of importPath after the last
// occurrence of the signature path elements
// (Godeps/_workspace/src) that always precede imported
// packages in rewritten import paths.
//
// For example,
//   unqualify(C)                         = C
//   unqualify(D/Godeps/_workspace/src/C) = C
func unqualify(importPath string) string {
	if i := strings.LastIndex(importPath, sep); i != -1 {
		importPath = importPath[i+len(sep):]
	}
	return importPath
}

// qualify qualifies importPath with its corresponding import
// path in the Godeps src copy of package pkg. If importPath
// is a directory lexically contained in a path in paths,
// it will be qualified with package pkg; otherwise, it will
// be returned unchanged.
//
// For example, given paths {D, T} and pkg C,
//   importPath  returns
//   C           C
//   fmt         fmt
//   D           C/Godeps/_workspace/src/D
//   D/P         C/Godeps/_workspace/src/D/P
//   T           C/Godeps/_workspace/src/T
func qualify(importPath, pkg string, paths []string) string {
	if containsPathPrefix(paths, importPath) {
		return pkg + sep + importPath
	}
	return importPath
}
