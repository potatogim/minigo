package main

import (
	"os"
	"strings"
)

var GENERATION int = 1

var allScopes map[identifier]*scope

var debugMode = false
var debugToken = false

var debugAst = false
var debugParser = false
var parseOnly = false
var resolveOnly = false
var exit = false

func printVersion() {
	println("minigo 0.1.0")
	println("Copyright (C) 2019 @DQNEO")
}

func parseOpts(args []string) []string {
	var r []string

	for _, opt := range args {
		if opt == "--version" {
			printVersion()
			exit = true
			return nil
		}
		if opt == "-t" {
			debugToken = true
		}
		if opt == "-a" {
			debugAst = true
		}
		if opt == "-p" {
			debugParser = true
		}
		if opt == "-d" {
			debugMode = true
		}
		if opt == "--parse-only" {
			parseOnly = true
		}
		if opt == "--resolve-only" {
			resolveOnly = true
		}
		if strings.HasSuffix(opt, ".go") {
			r = append(r, opt)
		} else if opt == "-" {
			return []string{"/dev/stdin"}
		}
	}

	return r
}

type pkgsource struct {
	name identifier
	code string
}

func parseStdPkg(p *parser, universe *scope, pkgname identifier, code string) *stdpkg {
	filename := string(pkgname) + ".memory"
	bs = NewByteStreamFromString(filename, code)

	// initialize a package
	p.methods = map[identifier]methods{}
	p.scopes[pkgname] = newScope(nil)
	p.currentPackageName = pkgname

	asf := p.parseSourceFile(bs, p.scopes[pkgname], false)

	p.resolve(universe)
	return &stdpkg{
		name:  pkgname,
		files: []*SourceFile{asf},
	}
}

func main() {
	var sourceFiles []string

	if len(os.Args) == 0 {
		println("ERROR: os.Args should not be empty")
		return
	}

	if len(os.Args) > 1 {
		sourceFiles = parseOpts(os.Args[1:len(os.Args)])
	}

	if exit {
		return
	}

	if len(sourceFiles) == 0 {
		println("No input files.")
		return
	}
	// parser to parse imported
	pForImport := &parser{}
	// parse imported only
	var imported map[identifier]bool = map[identifier]bool{}
	for _, sourceFile := range sourceFiles {
		s := readFile(sourceFile)
		bs := &ByteStream{
			filename:  sourceFile,
			source:    s,
			nextIndex: 0,
			line:      1,
			column:    0,
		}
		astFile := pForImport.parseSourceFile(bs, nil, true)
		for _, importDecl := range astFile.importDecls {
			for _, spec := range importDecl.specs {
				imported[getBaseNameFromImport(spec.path)] = true
			}
		}
	}

	var importOS bool
	_, importOS = imported["os"]

	// parse
	p := &parser{}

	p.methods = map[identifier]methods{}
	p.scopes = map[identifier]*scope{}

	allScopes = p.scopes

	var bs *ByteStream
	var astFiles []*SourceFile

	universe := newScope(nil)

	bs = NewByteStreamFromString("internalcode.memory", internalRuntimeCode)
	astFiles = append(astFiles, p.parseSourceFile(bs, universe, false))

	bs = NewByteStreamFromString("builtin.memory", builtinCode)
	astFiles = append(astFiles, p.parseSourceFile(bs, universe, false))

	setPredeclaredIdentifiers(universe)

	// add std packages
	var compiledPackages map[identifier]*stdpkg = map[identifier]*stdpkg{}
	// parse std packages
	for pkgName, _ := range imported {
		pkgCode, ok := stdPkgs[pkgName]
		if !ok {
			errorf("package '" + string(pkgName) + "' is not a standard library.")
		}
		pkg := parseStdPkg(p, universe, pkgName, pkgCode)
		compiledPackages[pkgName] = pkg
	}

	// initialize main package
	var pkgname identifier = "main"
	p.methods = map[identifier]methods{}
	p.scopes[pkgname] = newScope(nil)
	p.currentPackageName = pkgname

	for _, sourceFile := range sourceFiles {
		s := readFile(sourceFile)
		bs := &ByteStream{
			filename:  sourceFile,
			source:    s,
			nextIndex: 0,
			line:      1,
			column:    0,
		}
		asf := p.parseSourceFile(bs, p.scopes[pkgname], false)
		astFiles = append(astFiles, asf)
	}

	if parseOnly {
		return
	}
	p.resolve(universe)
	if resolveOnly {
		return
	}

	if debugAst {
		astFiles[len(astFiles)-1].dump()
	}

	var importedPackages []*stdpkg
	for _, compiledPkg := range compiledPackages {
		importedPackages = append(importedPackages, compiledPkg)
	}
	ir := ast2ir(importedPackages, astFiles, p.stringLiterals)

	var uniquedDynamicTypes map[string]int = map[string]int{}
	for _, gtype := range p.alldynamictypes {
		uniquedDynamicTypes[gtype.String()] = 0
	}
	ir.hashedTypes = uniquedDynamicTypes

	var typeId = 1 // start with 1 because we want to zero as error
	for _, concreteNamedType := range p.concreteNamedTypes {
		concreteNamedType.gtype.typeId = typeId
		//debugf("concreteNamedType: id=%d, name=%s", typeId, concreteNamedType.name)
		typeId++
	}

	var methodTable map[int][]string = map[int][]string{} // typeId : []methodTable
	for _, funcdecl := range ir.funcs {
		if funcdecl.receiver != nil {
			//debugf("funcdecl:%v", funcdecl)
			gtype := funcdecl.receiver.getGtype()
			if gtype.typ == G_POINTER {
				gtype = gtype.origType
			}
			if gtype.relation == nil {
				errorf("no relation for %#v", funcdecl.receiver.getGtype())
			}
			typeId := gtype.relation.gtype.typeId
			methodTable[typeId] = append(methodTable[typeId], string(funcdecl.getUniqueName()))
		}
	}
	//debugf("methodTable=%v", methodTable)
	ir.methodTable = methodTable
	ir.importOS = importOS

	ir.emit()
}

type stdpkg struct {
	name  identifier
	files []*SourceFile
}

func ast2ir(stdpkgs []*stdpkg, files []*SourceFile, stringLiterals []*ExprStringLiteral) *IrRoot {

	root := &IrRoot{
		stringLiterals: stringLiterals,
	}

	for _, pkg := range stdpkgs {
		for _, f := range pkg.files {
			for _, decl := range f.topLevelDecls {
				if decl.vardecl != nil {
					root.vars = append(root.vars, decl.vardecl)
				} else if decl.funcdecl != nil {
					//debugf("register func to ir:%v", decl.funcdecl)
					root.funcs = append(root.funcs, decl.funcdecl)
				}
			}
		}
	}

	for _, f := range files {
		for _, decl := range f.topLevelDecls {
			if decl.vardecl != nil {
				root.vars = append(root.vars, decl.vardecl)
			} else if decl.funcdecl != nil {
				//debugf("register func to ir:%v", decl.funcdecl)
				root.funcs = append(root.funcs, decl.funcdecl)
			}
		}
	}
	return root
}
