package main

import (
	"fmt"
	"runtime"

)

type parser struct {
	tokenStream *TokenStream
	packageBlockScope *scope
	currentScope *scope
	globalvars []*ExprVariable
	localvars []*ExprVariable
}

type TokenStream struct {
	tokens []*Token
	index  int
}

func (p *parser) peekToken() *Token {
	ts := p.tokenStream
	if ts.index > len(ts.tokens)-1 {
		return makeToken("EOF", "")
	}
	r := ts.tokens[ts.index]
	return r
}

func (p *parser) lastToken() *Token {
	ts := p.tokenStream
	return ts.tokens[ts.index - 1]
}

func (p *parser) readToken() *Token {
	ts := p.tokenStream
	if ts.index > len(ts.tokens)-1 {
		return makeToken("EOF", "")
	}
	r := ts.tokens[ts.index]
	ts.index++
	return r
}

func (p *parser) unreadToken() {
	p.tokenStream.index--
}

func (p *parser) readIdent() identifier {
	tok := p.readToken()
	if !tok.isTypeIdent() {
		errorf("Identifier expected, but got %s", tok)
	}
	return tok.getIdent()
}

func (p *parser) expectKeyword(name string) {
	tok := p.readToken()
	if !tok.isKeyword(name) {
		errorf("Keyword %s expected but got %s", tok)
	}
}

func (p *parser) expect(punct string) {
	tok := p.readToken()
	if !tok.isPunct(punct) {
		errorf("punct '%s' expected but got '%s'", punct, tok)
	}
}

func debugAstConstructed(ast interface{}) {
	debugPrintVar("Ast constructed", ast)
}

func getCallerName(n int) string {
	pc, _, _, ok := runtime.Caller(n)
	if !ok {
		errorf("Unable to get caller")
	}
	details := runtime.FuncForPC(pc)
	//r := (strings.Split(details.Name(), "."))[2]
	return details.Name()
}

func (p *parser) traceIn() int {
	if (!debugParser) {
		return 0
	}
	debugf("func %s start with %s", getCallerName(2), p.peekToken())
	debugNest++
	return 0
}

func (p *parser) traceOut(_ int) {
	if (!debugParser) {
		return
	}
	debugNest--
	debugf("func %s end", getCallerName(2))
}

func (p *parser) readFuncallArgs() []Expr {
	defer p.traceOut(p.traceIn())
	var r []Expr
	for {
		tok := p.readToken()
		if tok.isPunct(")") {
			return r
		}
		p.unreadToken()
		arg := p.parseExpr()
		r = append(r, arg)
		tok = p.readToken()
		if tok.isPunct(")") {
			return r
		} else if tok.isPunct(",") {
			continue
		} else {
			errorf("invalid token in funcall arguments: %s", tok)
		}
	}
}

func (p *parser) parseIdentOrFuncall(firstIdent identifier) Expr {
	defer p.traceOut(p.traceIn())

	// https://golang.org/ref/spec#QualifiedIdent
	// read QualifiedIdent
	tok := p.readToken()
	var pkg identifier
	var ident identifier
	if tok.isPunct(".") {
		pkg = firstIdent
		ident = p.readIdent()
	} else {
		p.unreadToken()
		pkg = ""
		ident = firstIdent
	}

	operand := &AstOperandName{
		pkg:   pkg,
		ident: ident,
	}

	tok = p.readToken()
	if tok.isPunct("(") {
		// try funcall
		args := p.readFuncallArgs()

		if operand.pkg == "" && operand.ident == "println" {
			// dirty hack: "println" -> "puts"
			operand.ident = "puts"
		} else if operand.pkg == "fmt" && operand.ident == "Printf" {
			// dirty hack: "fmt" -> "Printf"
			operand.ident = "printf"
		}
		return &ExprFuncall{
			fname: operand.ident,
			args:  args,
		}
	} else if tok.isPunct("[") {
		// index access
		// assure operand is array, slice, or map
		tok := p.readToken()
		if tok.isPunct(":") {
			lowIndex := &ExprNumberLiteral{
				val: 0,
			}
			highIndex := p.parseExpr()
			p.expect("]")
			return &ExprSliced{
				ref:  operand,
				low:  lowIndex,
				high: highIndex,
			}
		} else {
			p.unreadToken()
			index := p.parseExpr()
			tok := p.readToken()
			if tok.isPunct("]") {
				return &ExprIndexAccess{
					ref:   operand,
					index: index,
				}
			} else if tok.isPunct(":") {
				highIndex := p.parseExpr()
				p.expect("]")
				return &ExprSliced{
					ref:  operand,
					low:  index,
					high: highIndex,
				}

			} else {
				tok.errorf("invalid token in index access")
			}
		}
	} else {
		p.unreadToken()
	}

	v := p.currentScope.get(firstIdent)
	if v == nil {
		errorf("Undefined variable: %s", firstIdent)
		return nil
	}
	vardecl, _ := v.(*AstVarDecl)
	if vardecl != nil {
		return vardecl.variable
	}
	constdecl, _ := v.(*AstConstDecl)
	if constdecl != nil {
		return constdecl.variable
	}
	errorf("variable not found %v", firstIdent)
	return nil
}

var stringIndex = 0
var stringLiterals []*ExprStringLiteral

func (p *parser) newAstString(sval string) *ExprStringLiteral {
	ast := &ExprStringLiteral{
		val:    sval,
		slabel: fmt.Sprintf("L%d", stringIndex),
	}
	stringIndex++
	stringLiterals = append(stringLiterals, ast)
	return ast
}

func (p *parser) parsePrim() Expr {
	defer p.traceOut(p.traceIn())
	tok := p.readToken()
	switch {
	case tok.isTypeIdent():
		return p.parseIdentOrFuncall(tok.getIdent())
	case tok.isTypeString():
		return p.newAstString(tok.sval)
	case tok.isTypeInt():
		ival := tok.getIntval()
		return &ExprNumberLiteral{
			val: ival,
		}
	case tok.isPunct("["):
		return p.parseArrayLiteral()
	default:
		errorf("unable to handle %s", tok)
	}
	return nil
}

func (p *parser) parseArrayLiteral() Expr {
	defer p.traceOut(p.traceIn())

	p.expect("]")
	typ := p.parseType()
	p.expect("{")
	var values []Expr
	for {
		tok := p.readToken()
		if tok.isPunct("}") {
			break
		}
		var v Expr
		if tok.isTypeString() {
			v = &ExprStringLiteral{val: tok.sval}
		} else if tok.isTypeInt() {
			v = &ExprNumberLiteral{val: tok.getIntval()}
		}
		tok = p.readToken()
		if tok.isPunct(",") {
			continue
		} else if tok.isPunct("}") {
			break
		} else {
			errorf("unpexpected %s", tok)
		}
		values = append(values, v)
	}

	gtype := &Gtype{
		typ:    "array",
		length: len(values),
		ptr:    typ,
	}

	r := &ExprArrayLiteral{
		gtype:  gtype,
		values: values,
	}
	debugAstConstructed(r)
	return r
}

func (p *parser) parseUnaryExpr() Expr {
	return p.parsePrim()
}

func priority(op string) int {
	switch op {
	case "==", "!=", "<", ">", ">=", "<=":
		return 10
	case "-", "+":
		return 10
	case "*":
		return 20
	default:
		errorf("unkown operator %s", op)
	}
	return 0
}

func (p *parser) parseExpr() Expr {
	return p.parseExprInt(-1)
}

var binops = []string{
	"+", "*", "-", "==", "!=", "<", ">", "<=", "=>",
}

func (p *parser) parseExprInt(prior int) Expr {
	defer p.traceOut(p.traceIn())

	ast := p.parseUnaryExpr()
	debugAstConstructed(ast)
	if ast == nil {
		return nil
	}
	for {
		tok := p.readToken()
		if tok.isSemicolon() {
			p.unreadToken()
			return ast
		}

		// if bion
		if in_array(tok.sval, binops) {
			prior2 := priority(tok.sval)
			if prior < prior2 {
				right := p.parseExprInt(prior2)
				if ast == nil {
					tok.errorf("bad left unary expr:%v", ast)
				}
				ast = &ExprBinop{
					op:    tok.sval,
					left:  ast,
					right: right,
				}
				debugAstConstructed(ast)
				continue
			} else {
				p.unreadToken()
				return ast
			}
			/*
				} else if tok.sval == "," || tok.sval == ")" ||
					tok.sval == "{" || tok.sval == "}" ||
					tok.isPunct(";") || tok.isPunct(":") { // end of funcall argument
					p.unreadToken()
					return ast
			*/
		} else {
			p.unreadToken()
			return ast
			tok.errorf("Unexpected")
		}
	}

	return ast
}

func (p *parser) newVariable(varname identifier, gtype *Gtype, isGlobal bool) *ExprVariable {
	var variable *ExprVariable
	if isGlobal {
		variable = &ExprVariable{
			varname:  varname,
			gtype:    gtype,
			isGlobal: true,
		}
		p.globalvars = append(p.globalvars, variable)
	} else {
		variable = &ExprVariable{
			varname: varname,
			gtype:   gtype,
		}
		p.localvars = append(p.localvars, variable)
	}
	return variable
}

// https://golang.org/ref/spec#Type
func (p *parser) parseType() *Gtype {
	defer p.traceOut(p.traceIn())
	for {
		tok := p.readToken()
		if tok.isPunct("*") {
			// pointer
		} else if tok.isKeyword("struct") {

		} else if tok.isTypeIdent() {
			//_ := tok.getIdent()
			//_ := p.p.currentScope.getGtype(typename)
			//return gtype
		} else if tok.isPunct("[") {
		} else if tok.isPunct("]") {

		} else {
			p.unreadToken()
			break
		}

	}
	return gInt // FIXME
}

func (p *parser) parseDeclVar(isGlobal bool) *AstVarDecl {
	assert(p.lastToken().isKeyword("var"),`require "var" is already read`)
	defer p.traceOut(p.traceIn())
	// read name
	name := p.readIdent()

	// "=" or Type
	tok := p.readToken()
	//var gtype *Gtype
	var initval Expr
	if tok.isPunct("=") {
		//var x = EXPR
		initval = p.parseExpr()
		//gtype = gInt  // FIXME: infer type
		p.expect(";")
	} else {
		p.unreadToken()
		// expect Type
		_ = p.parseType()
		tok3 := p.readToken()
		if tok3.isPunct("=") {
			initval = p.parseExpr()
			p.expect(";")
		} else if tok3.isPunct(";") {
			// k
		} else {
			errorf("Invalid token %s", tok3)
		}
	}

	variable := p.newVariable(name, nil, isGlobal)

	r := &AstVarDecl{
		variable: variable,
		initval:  initval,
	}
	p.currentScope.setVarDecl(name, r)
	return r
}

func (p *parser) parseConstDecl() *AstConstDecl {
	defer p.traceOut(p.traceIn())
	// read name
	name := p.readIdent()

	// Type or "="
	tok := p.readToken()
	var val Expr
	if tok.isPunct("=") {
		// infer mode: const x = EXPR
		val = p.parseExpr()
		p.expect(";")
	} else {
		p.unreadToken()
		// expect Type
		_ = p.parseType()
		// const x T = EXPR
		p.expect("=")
		val = p.parseExpr()
		p.expect(";")
	}

	variable := &ExprConstVariable{
		name: name,
		val:  val,
	}

	r := &AstConstDecl{
		variable: variable,
	}

	p.currentScope.setConstDecl(name, r)
	return r
}

func (p *parser) parseAssignment() *AstAssignment {
	defer p.traceOut(p.traceIn())
	tleft := p.readToken()
	item := p.currentScope.get(tleft.getIdent())
	if item == nil {
		errorf("variable %s is not found", tleft.getIdent())
	}
	vardecl, ok := item.(*AstVarDecl)
	if !ok {
		errorf("%s is not a variable", tleft)
	}
	p.expect("=")
	rexpr := p.parseExpr()
	p.expect(";")
	return &AstAssignment{
		left:  vardecl.variable,
		right: rexpr,
	}
}

func (p *parser) parseIdentList() []identifier {
	defer p.traceOut(p.traceIn())
	var r []identifier
	for {
		tok := p.readToken()
		if tok.isTypeIdent() {
			r = append(r, tok.getIdent())
		} else if len(r) == 0 {
			// at least one ident is needed
			tok.errorf("Ident expected")
		}

		tok = p.readToken()
		if tok.isPunct(",") {
			continue
		} else {
			p.unreadToken()
			return r
		}
	}
	return r
}

func (p *parser) enterNewScope() {
	p.currentScope = newScope(p.currentScope)
}

func (p *parser) exitScope() {
	p.currentScope = p.currentScope.outer
}

func (p *parser) parseForStmt() *AstForStmt {
	defer p.traceOut(p.traceIn())
	var r = &AstForStmt{}
	p.enterNewScope()
	defer p.exitScope()
	// Assume "range" style
	idents := p.parseIdentList()
	p.expect(":=")
	for _, ident := range idents {
		p.currentScope.setVarDecl(ident, &AstVarDecl{variable: p.newVariable(ident, nil, false)})
	}
	r.idents = idents
	p.expectKeyword("range")
	r.list = p.parseExpr()
	p.expect("{")
	r.block = p.parseCompoundStmt()
	return r
}

func (p *parser) parseIfStmt() *AstIfStmt {
	defer p.traceOut(p.traceIn())
	var r = &AstIfStmt{}
	p.enterNewScope()
	defer p.exitScope()
	r.cond = p.parseExpr()
	p.expect("{")
	r.then = p.parseCompoundStmt()
	tok := p.readToken()
	if tok.isKeyword("else") {
		tok := p.readToken()
		if tok.isPunct("{") {
			r.els = &AstStmt{compound: p.parseCompoundStmt()}
		} else if tok.isKeyword("if") {
			r.els = &AstStmt{ifstmt: p.parseIfStmt()}
		} else {
			tok.errorf("Syntax error")
		}
	} else {
		p.unreadToken()
	}
	return r
}

func (p *parser) parseStmt() *AstStmt {
	defer p.traceOut(p.traceIn())
	tok := p.readToken()
	if tok.isKeyword("var") {
		return &AstStmt{declvar: p.parseDeclVar(false)}
	} else if tok.isKeyword("const") {
		return &AstStmt{constdecl: p.parseConstDecl()}
	} else if tok.isKeyword("type") {
		return &AstStmt{typedecl: p.parseTypeDecl()}
	} else if tok.isKeyword("for") {
		return &AstStmt{forstmt: p.parseForStmt()}
	} else if tok.isKeyword("if") {
		return &AstStmt{ifstmt: p.parseIfStmt()}
	}
	tok2 := p.readToken()

	if tok2.isPunct("=") {
		p.unreadToken()
		p.unreadToken()
		//assure_lvalue(ast)
		return &AstStmt{assignment: p.parseAssignment()}
	}
	p.unreadToken()
	p.unreadToken()
	return &AstStmt{expr: p.parseExpr()}
}

func (p *parser) parseCompoundStmt() *AstCompountStmt {
	defer p.traceOut(p.traceIn())

	r := &AstCompountStmt{}
	for {
		tok := p.readToken()
		if tok.isPunct("}") {
			return r
		}
		if tok.isSemicolon() {
			continue
		}
		p.unreadToken()
		stmt := p.parseStmt()
		r.stmts = append(r.stmts, stmt)
	}
	return nil
}

func (p *parser) parseFuncDef() *AstFuncDecl {
	defer p.traceOut(p.traceIn())
	p.localvars = make([]*ExprVariable, 0)
	p.enterNewScope()
	defer p.exitScope()
	fname := p.readToken().getIdent()
	p.expect("(")
	var params []*ExprVariable

	tok := p.readToken()
	if !tok.isPunct(")") {
		p.unreadToken()
		for {
			tok := p.readToken()
			pname := tok.getIdent()
			ptype := p.parseType()
			// assureType(tok.sval)
			variable := &ExprVariable{
				varname: pname,
				gtype:   ptype,
			}
			params = append(params, variable)
			p.currentScope.setVarDecl(pname, &AstVarDecl{
				variable: variable,
			})
			tok = p.readToken()
			if tok.isPunct(")") {
				break
			}
			if !tok.isPunct(",") {
				errorf("Invalid token %s", tok)
			}
		}
	}

	// read func rettype
	tok = p.readToken()
	var rettype string
	if tok.isTypeIdent() {
		// rettype
		rettype = tok.sval
		p.expect("{")
	} else {
		assert(tok.isPunct("{"), "begin of func body")
	}
	debugf("scope:%s", p.currentScope)
	body := p.parseCompoundStmt()
	r := &AstFuncDecl{
		fname:     fname,
		rettype:   rettype,
		params:    params,
		localvars: p.localvars,
		body:      body,
	}
	p.localvars = nil
	return r
}

func (p *parser) parseImport() *AstImportDecl {
	defer p.traceOut(p.traceIn())
	tok := p.readToken()
	var paths []string
	if tok.isPunct("(") {
		for {
			tok := p.readToken()
			if tok.isTypeString() {
				paths = append(paths, tok.sval)
				p.expect(";")
			} else if tok.isPunct(")") {
				break
			} else {
				errorf("invalid import path %s", tok)
			}
		}
	} else {
		if !tok.isTypeString() {
			errorf("import expects package name")
		}
		paths = []string{tok.sval}
	}

	p.expect(";")
	return &AstImportDecl{
		paths: paths,
	}
}

func (p *parser) expectPackageClause() *AstPackageClause {
	defer p.traceOut(p.traceIn())
	p.expectKeyword("package")
	r := &AstPackageClause{name: p.readIdent()}
	p.expect(";")
	return r
}

func (p *parser) parseImportDecls() []*AstImportDecl {
	defer p.traceOut(p.traceIn())
	var r []*AstImportDecl
	for {
		tok := p.readToken()
		if !tok.isKeyword("import") {
			p.unreadToken()
			return r
		}
		r = append(r, p.parseImport())
	}
}

// read after "struct" token
func (p *parser) parseStructDef() *AstStructDef {
	assert(p.lastToken().isKeyword("struct"),`require "struct" is already read`)
	defer p.traceOut(p.traceIn())
	p.expect("{")
	var fields []*StructField
	for {
		tok := p.readToken()
		if tok.isPunct("}") {
			break
		}
		fieldname := tok.getIdent()
		fieldtyep := p.parseType()
		fields = append(fields, &StructField{
			name:  fieldname,
			gtype: fieldtyep,
		})
		p.expect(";")
	}
	p.expect(";")
	return &AstStructDef{
		fields: fields,
	}
}

func (p *parser) parseInterfaceDef() *AstInterfaceDef {
	defer p.traceOut(p.traceIn())
	p.expect("{")
	var methods []identifier
	for {
		tok := p.readToken()
		if tok.isPunct("}") {
			break
		}
		fname := tok.getIdent()
		p.expect("(")
		p.expect(")")
		p.expect(";")
		methods = append(methods, fname)
	}
	p.expect(";")
	return &AstInterfaceDef{
		methods: methods,
	}
}

func (p *parser) parseTypeDecl() *AstTypeDecl {
	defer p.traceOut(p.traceIn())
	name := p.readIdent()
	tok := p.readToken()
	var typeConstuctor interface{}
	if tok.isKeyword("struct") {
		typeConstuctor = p.parseStructDef()
	} else if tok.isKeyword("interface") {
		typeConstuctor = p.parseInterfaceDef()
	} else if tok.isTypeIdent() {
		ident := tok.getIdent() // name of another type
		typeConstuctor = ident
	} else {
		tok.errorf("TBD")
	}
	r := &AstTypeDecl{
		typedef: &AstTypeDef{
			name:            name,
			typeConstructor: typeConstuctor,
		},
	}
	p.currentScope.setTypeDecl(name, r)
	return r
}

func (p *parser) parseTopLevelDecl(tok *Token) *AstTopLevelDecl {
	defer p.traceOut(p.traceIn())
	var r *AstTopLevelDecl
	switch {
	case tok.isKeyword("var"):
		vardecl := p.parseDeclVar(true)
		r = &AstTopLevelDecl{vardecl: vardecl}
	case tok.isKeyword("const"):
		constdecl := p.parseConstDecl()
		r = &AstTopLevelDecl{constdecl: constdecl}
	case tok.isKeyword("func"):
		funcdecl := p.parseFuncDef()
		r = &AstTopLevelDecl{funcdecl: funcdecl}
	case tok.isKeyword("type"):
		typedecl := p.parseTypeDecl()
		r = &AstTopLevelDecl{typedecl: typedecl}
	default:
		errorf("TBD: unable to handle token %v", tok)
	}

	debugAstConstructed(r)
	return r
}

func (p *parser) parseTopLevelDecls() []*AstTopLevelDecl {
	defer p.traceOut(p.traceIn())
	var r []*AstTopLevelDecl
	for {
		tok := p.readToken()
		if tok.isEOF() {
			return r
		}

		if tok.isPunct(";") {
			continue
		}
		ast := p.parseTopLevelDecl(tok)
		r = append(r, ast)
	}
	return r
}

// https://golang.org/ref/spec#Source_file_organization
// Each source file consists of
// a package clause defining the package to which it belongs,
// followed by a possibly empty set of import declarations that declare packages whose contents it wishes to use,
// followed by a possibly empty set of declarations of functions, types, variables, and constants.
func (p *parser) parseSourceFile(sourceFile string, packageBlockScope *scope) *AstSourceFile {

	// tokenize
	bs := NewByteStream(sourceFile)
	tokens := tokenize(bs)
	assert(len(tokens) > 0, "tokens should have length")

	/*
	if debugToken {
		renderTokens(tokens)
	}
	*/

	p.tokenStream = &TokenStream{
		tokens: tokens,
		index:  0,
	}

	p.packageBlockScope = packageBlockScope
	p.currentScope = packageBlockScope

	r := &AstSourceFile{}
	r.pkg = p.expectPackageClause()
	r.imports = p.parseImportDecls()

	r.packageNames = make(map[identifier]string)
	for _, importdecl := range r.imports {
		for _, path := range importdecl.paths {
			ident := identifier(path)
			r.packageNames[ident] = path
		}
	}

	r.decls = p.parseTopLevelDecls()
	return r
}

type resolver struct {
	packageblockscope *scope
}

func (r *resolver) resolveVar(decl *AstVarDecl) {
	if decl.variable.gtype != nil {
		return
	}

	constructor := decl.variable.typeConstructor
	switch constructor.(type) {
	case identifier:
		item := r.packageblockscope.get(constructor.(identifier))
		if item == nil {
			errorf("Undefined type %v", item)
		}
		typedecl, ok := item.(*AstTypeDecl)
		if !ok {
			errorf("%v is not a type", item)
		}
		if typedecl.gtype == nil {
			errorf("type is not resolved", item)
		}
		decl.variable.gtype = typedecl.gtype
	}
}

func (r *resolver) resolveConst(decl *AstConstDecl) {
	if decl.variable.gtype != nil {
		return
	}

	constructor := decl.variable.typeConstructor
	switch constructor.(type) {
	case identifier:
		item := r.packageblockscope.get(constructor.(identifier))
		if item == nil {
			errorf("Undefined type %v", item)
		}
		typedecl, ok := item.(*AstTypeDecl)
		if !ok {
			errorf("%v is not a type", item)
		}
		if typedecl.gtype == nil {
			errorf("type is not resolved", item)
		}
		decl.variable.gtype = typedecl.gtype
	}
}

func (r *resolver) resolveType(decl *AstTypeDecl) {
	if decl.gtype != nil {
		return
	}

	constructor := decl.typedef.typeConstructor
	switch constructor.(type) {
	case identifier:
		item := r.packageblockscope.get(constructor.(identifier))
		if item == nil {
			errorf("Undefined type %v", item)
		}
		typedecl, ok := item.(*AstTypeDecl)
		if !ok {
			errorf("%v is not a type", item)
		}
		if typedecl.gtype == nil {
			r.resolveType(typedecl)
		}
		decl.gtype = &Gtype{
			typ:  "ref",
			size: typedecl.gtype.size,
			ptr:  typedecl.gtype,
		}

	}
}

//TODO: resolve local scope
func (r *resolver) resolve() {
	for _, decl := range r.packageblockscope.idents {
		typedecl, ok := decl.(*AstTypeDecl)
		if ok {
			r.resolveType(typedecl)
			typedecl.dump()
		}
	}

	for _, decl := range r.packageblockscope.idents {
		constdecl, ok := decl.(*AstConstDecl)
		if ok {
			r.resolveConst(constdecl)
			constdecl.dump()
		}
	}

	for _, decl := range r.packageblockscope.idents {
		vardecl, ok := decl.(*AstVarDecl)
		if ok {
			debugf("resolve decl :")
			r.resolveVar(vardecl)
			vardecl.dump()
		}
	}
}
