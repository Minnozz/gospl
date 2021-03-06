package parser

import (
	"github.com/Minnozz/gospl/ast"
	"github.com/Minnozz/gospl/scanner"
	"github.com/Minnozz/gospl/token"
)

type Parser struct {
	Errors scanner.ErrorList

	fileInfo *token.FileInfo
	scanner  scanner.Scanner

	comments []*ast.Comment

	// Current scanner token
	pos token.Pos
	tok token.Token
	lit string
}

func (p *Parser) Init(fileInfo *token.FileInfo, src []byte) {
	p.fileInfo = fileInfo
	p.scanner.Init(fileInfo, src, func(pos token.Position, msg string) {
		p.Errors.Add(pos, msg)
	})

	p.next()
}
func (p *Parser) Parse() *ast.File {
	var declarations []ast.Declaration
	for p.tok != token.EOF {
		declarations = append(declarations, p.parseDeclaration())
	}

	return &ast.File{
		Declarations: declarations,
		Comments:     p.comments,
	}
}

func (p *Parser) nextToken() {
	// Advance scanner to next token
	p.pos, p.tok, p.lit = p.scanner.Scan()
}

func (p *Parser) next() {
	p.nextToken()

	// Automatically parse comments outside of the normal AST
	for p.tok == token.COMMENT {
		p.comments = append(p.comments, p.parseComment())
	}
}

func (p *Parser) parseComment() *ast.Comment {
	pos := p.pos
	var text string
	if p.tok == token.COMMENT {
		text = p.lit
	} else {
		p.errorExpected(p.pos, "comment")
	}
	// Don't call p.next() because we are already in the for loop inside next().
	p.nextToken()

	return &ast.Comment{
		TextPos: pos,
		Text:    text,
	}
}

func (p *Parser) error(pos token.Pos, msg string) {
	position := p.fileInfo.Position(pos)
	p.Errors.Add(position, msg)
}

func (p *Parser) errorExpected(pos token.Pos, what string) {
	if p.pos == pos {
		p.error(pos, "expected "+what+", got "+p.tok.String())
	} else {
		p.error(pos, "expected "+what)
	}
}

func (p *Parser) expect(tok token.Token) token.Pos {
	pos := p.pos
	if p.tok != tok {
		p.errorExpected(pos, tok.String())
	}
	p.next()
	return pos
}

func (p *Parser) parseDeclaration() ast.Declaration {
	pos := p.pos
	t := p.parseType()
	name := p.parseIdentifier()

	switch p.tok {
	case token.IS:
		return p.continueVariableDeclaration(t, name)
	case token.ROUND_BRACKET_OPEN:
		return p.continueFunctionDeclaration(t, name)
	default:
		p.errorExpected(p.pos, "declaration")
		p.next()
		return &ast.BadDeclaration{
			From: pos,
			To:   p.pos,
		}
	}
}

func (p *Parser) parseType() ast.Type {
	pos := p.pos

	switch p.tok {
	case token.IDENTIFIER:
		name := p.parseIdentifier()

		return &ast.NamedType{
			Name: name,
		}
	case token.ROUND_BRACKET_OPEN:
		p.next()
		left := p.parseType()
		p.expect(token.COMMA)
		right := p.parseType()
		end := p.expect(token.ROUND_BRACKET_CLOSE)

		return &ast.TupleType{
			RoundBracketOpen:  pos,
			Left:              left,
			Right:             right,
			RoundBracketClose: end,
		}
	case token.SQUARE_BRACKET_OPEN:
		p.next()
		el := p.parseType()
		end := p.expect(token.SQUARE_BRACKET_CLOSE)

		return &ast.ListType{
			SquareBracketOpen:  pos,
			ElementType:        el,
			SquareBracketClose: end,
		}
	default:
		p.errorExpected(p.pos, "type")
		p.next()

		return &ast.BadType{
			From: pos,
			To:   p.pos,
		}
	}
}

func (p *Parser) parseIdentifier() *ast.Identifier {
	pos := p.pos

	var name string
	if p.tok == token.IDENTIFIER {
		name = p.lit
		p.next()
	} else {
		p.expect(token.IDENTIFIER)
	}

	return &ast.Identifier{
		NamePos: pos,
		Name:    name,
	}
}

func (p *Parser) continueVariableDeclaration(t ast.Type, name *ast.Identifier) *ast.VariableDeclaration {
	p.expect(token.IS)
	initializer := p.parseExpression()
	end := p.expect(token.SEMICOLON)

	return &ast.VariableDeclaration{
		Type:        t,
		Name:        name,
		Initializer: initializer,
		Semicolon:   end,
	}
}

func (p *Parser) parseExpression() ast.Expression {
	return p.parseExpressionWithMinPrecedence(0)
}

func (p *Parser) parseExpressionWithMinPrecedence(minPrec Precedence) ast.Expression {
	// Parse initial leg of expression
	expr := p.parseUnaryExpression()

	// If the next token is a binary operator, expr will become the lhs of that binary expression unless its operator precedence is
	// lower than the current minPrec.
precedenceGroup:
	for p.tok != token.EOF {
		switch p.tok {
		case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO,
			token.EQUALS, token.LESS_THAN, token.GREATER_THAN, token.LESS_THAN_EQUALS, token.GREATER_THAN_EQUALS, token.NOT_EQUALS,
			token.AND, token.OR, token.COLON:
			// Next token is a binary operator
			prec, assoc := binaryPrecAssoc(p.tok)
			if prec < minPrec {
				// Operator precedence is too low for this precedence group.  This expr will become the lhs of the next binary
				// expression in an enclosing call to parseExpressionWithMinPrecedence().
				break precedenceGroup
			}

			op := p.tok
			p.next()

			newMinPrec := prec
			if assoc == LeftAssociative {
				// Even if the next binary expression has the same precedence as the current one, it should not be parsed into the
				// rhs of this expression because of left associativity.
				// Instead, this expr will become the lhs of the next binary expression in the next iteration of the precedenceGroup
				// loop (or in an enclosing call to parseExpressionWithMinPrecedence()).
				newMinPrec += 1
			}
			rhs := p.parseExpressionWithMinPrecedence(newMinPrec)

			expr = &ast.BinaryExpression{
				Left:     expr,
				Operator: op,
				Right:    rhs,
			}
		default:
			// expr is not part of a binary expression
			break precedenceGroup
		}
	}

	return expr
}

func (p *Parser) parseUnaryExpression() ast.Expression {
	pos := p.pos

	switch p.tok {
	case token.INTEGER, token.EMPTY_LIST:
		return p.parseLiteralExpression()

	case token.IDENTIFIER:
		ident := p.parseIdentifier()

		if p.tok == token.ROUND_BRACKET_OPEN {
			// Function call
			return p.continueFunctionCallExpression(ident)
		}

		// Identifier
		return ident

	case token.ROUND_BRACKET_OPEN:
		p.next()
		expr := p.parseExpression()

		if p.tok == token.COMMA {
			// Tuple expression
			p.next()
			second := p.parseExpression()
			end := p.expect(token.ROUND_BRACKET_CLOSE)

			return &ast.TupleExpression{
				RoundBracketOpen:  pos,
				Left:              expr,
				Right:             second,
				RoundBracketClose: end,
			}
		}

		// Parenthesized expression
		end := p.expect(token.ROUND_BRACKET_CLOSE)

		return &ast.ParenthesizedExpression{
			RoundBracketOpen:  pos,
			Expression:        expr,
			RoundBracketClose: end,
		}

	case token.MINUS, token.NOT:
		minPrec, assoc := unaryPrecAssoc(p.tok)
		if assoc == LeftAssociative {
			minPrec += 1
		}

		op := p.tok
		p.next()

		operand := p.parseExpressionWithMinPrecedence(minPrec)

		return &ast.UnaryExpression{
			OperatorPos: pos,
			Operator:    op,
			Operand:     operand,
		}

	default:
		p.errorExpected(p.pos, "unary expression")
		p.next()

		return &ast.BadExpression{
			From: pos,
			To:   p.pos,
		}
	}
}

func (p *Parser) parseLiteralExpression() *ast.LiteralExpression {
	pos := p.pos

	switch p.tok {
	case token.INTEGER, token.EMPTY_LIST:
		kind, value := p.tok, p.lit
		p.next()

		return &ast.LiteralExpression{
			ValuePos: pos,
			Kind:     kind,
			Value:    value,
		}
	default:
		p.errorExpected(p.pos, "literal expression")
		p.next()

		return &ast.LiteralExpression{
			ValuePos: pos,
			Kind:     token.INVALID,
			Value:    "",
		}
	}
}

func (p *Parser) continueFunctionCallExpression(name *ast.Identifier) *ast.FunctionCallExpression {
	p.expect(token.ROUND_BRACKET_OPEN)

	var args []ast.Expression

	if p.tok != token.ROUND_BRACKET_CLOSE {
	arguments:
		for {
			args = append(args, p.parseExpression())

			switch p.tok {
			case token.COMMA:
				p.next()
			case token.ROUND_BRACKET_CLOSE:
				break arguments
			default:
				p.errorExpected(p.pos, token.COMMA.String()+" or "+token.ROUND_BRACKET_CLOSE.String())
				p.next()
				break arguments
			}
		}
	}

	end := p.expect(token.ROUND_BRACKET_CLOSE)

	return &ast.FunctionCallExpression{
		Name:              name,
		Arguments:         args,
		RoundBracketClose: end,
	}
}

func (p *Parser) continueFunctionDeclaration(returnType ast.Type, name *ast.Identifier) *ast.FunctionDeclaration {
	params := p.parseFunctionParameters()

	varDecls, stmts, end := p.parseFunctionBody()

	return &ast.FunctionDeclaration{
		ReturnType:        returnType,
		Name:              name,
		Parameters:        params,
		Variables:         varDecls,
		Statements:        stmts,
		CurlyBracketClose: end,
	}
}

func (p *Parser) parseFunctionParameters() *ast.FunctionParameters {
	pos := p.expect(token.ROUND_BRACKET_OPEN)

	var params []*ast.FunctionParameter

	if p.tok != token.ROUND_BRACKET_CLOSE {
	parameters:
		for p.tok != token.EOF {
			params = append(params, p.parseFunctionParameter())

			switch p.tok {
			case token.COMMA:
				p.next()
			case token.ROUND_BRACKET_CLOSE:
				break parameters
			default:
				p.errorExpected(p.pos, token.COMMA.String()+" or "+token.ROUND_BRACKET_CLOSE.String())
				p.next()
				break parameters
			}
		}
	}

	end := p.expect(token.ROUND_BRACKET_CLOSE)

	return &ast.FunctionParameters{
		RoundBracketOpen:  pos,
		Parameters:        params,
		RoundBracketClose: end,
	}
}

func (p *Parser) parseFunctionParameter() *ast.FunctionParameter {
	t := p.parseType()
	name := p.parseIdentifier()

	return &ast.FunctionParameter{
		Type: t,
		Name: name,
	}
}

func (p *Parser) parseFunctionBody() ([]*ast.VariableDeclaration, []ast.Statement, token.Pos) {
	p.expect(token.CURLY_BRACKET_OPEN)

	var varDecls []*ast.VariableDeclaration
	var stmts []ast.Statement

	allowVardecl := true
	for p.tok != token.CURLY_BRACKET_CLOSE && p.tok != token.EOF {
		varDecl, stmt := p.parseVariableDeclarationOrStatement(allowVardecl)
		if varDecl != nil {
			varDecls = append(varDecls, varDecl)
		} else {
			stmts = append(stmts, stmt)
			allowVardecl = false
		}
	}

	end := p.expect(token.CURLY_BRACKET_CLOSE)

	return varDecls, stmts, end
}

func (p *Parser) parseVariableDeclarationOrStatement(allowVariableDeclaration bool) (*ast.VariableDeclaration, ast.Statement) {
	switch p.tok {
	case token.RETURN:
		return nil, p.parseReturnStatement()
	case token.IF:
		return nil, p.parseIfStatement()
	case token.CURLY_BRACKET_OPEN:
		return nil, p.parseBlockStatement()
	case token.IDENTIFIER:
		ident := p.parseIdentifier()

		// Possible statements
		switch p.tok {
		case token.IS:
			return nil, p.continueAssignmentStatement(ident)
		case token.ROUND_BRACKET_OPEN:
			return nil, p.continueFunctionCallStatement(ident)
		}

		if !allowVariableDeclaration {
			p.errorExpected(p.pos, "assignment or function call")
			p.next()
			return nil, &ast.BadStatement{}
		}

		// Variable declaration with type ident
		t := &ast.NamedType{
			Name: ident,
		}
		name := p.parseIdentifier()
		return p.continueVariableDeclaration(t, name), nil
	case token.ROUND_BRACKET_OPEN, token.SQUARE_BRACKET_OPEN:
		if !allowVariableDeclaration {
			p.errorExpected(p.pos, "statement")
			p.next()
			return nil, &ast.BadStatement{}
		}
		t := p.parseType()
		name := p.parseIdentifier()
		return p.continueVariableDeclaration(t, name), nil
	case token.WHILE:
		return nil, p.parseWhileStatement()
	default:
		if allowVariableDeclaration {
			p.errorExpected(p.pos, "variable declaration or statement")
		} else {
			p.errorExpected(p.pos, "statement")
		}
		p.next()
		return nil, &ast.BadStatement{}
	}
}

func (p *Parser) parseStatement() ast.Statement {
	_, stmt := p.parseVariableDeclarationOrStatement(false)
	return stmt
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	pos := p.expect(token.RETURN)

	var expr ast.Expression
	if p.tok == token.SEMICOLON {
		// Empty return statement (not allowed according to grammar)
	} else {
		expr = p.parseExpression()
	}

	end := p.expect(token.SEMICOLON)

	return &ast.ReturnStatement{
		Return:    pos,
		Value:     expr,
		Semicolon: end,
	}
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	pos := p.expect(token.IF)
	p.expect(token.ROUND_BRACKET_OPEN)

	cond := p.parseExpression()

	p.expect(token.ROUND_BRACKET_CLOSE)

	body := p.parseStatement()

	var elseStmt ast.Statement
	if p.tok == token.ELSE {
		p.next()
		elseStmt = p.parseStatement()
	}

	return &ast.IfStatement{
		If:        pos,
		Condition: cond,
		Body:      body,
		Else:      elseStmt,
	}
}

func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	pos := p.expect(token.WHILE)
	p.expect(token.ROUND_BRACKET_OPEN)

	cond := p.parseExpression()

	p.expect(token.ROUND_BRACKET_CLOSE)

	body := p.parseStatement()

	return &ast.WhileStatement{
		While:     pos,
		Condition: cond,
		Body:      body,
	}
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	pos := p.expect(token.CURLY_BRACKET_OPEN)

	var stmts []ast.Statement
	for p.tok != token.CURLY_BRACKET_CLOSE && p.tok != token.EOF {
		stmts = append(stmts, p.parseStatement())
	}

	end := p.expect(token.CURLY_BRACKET_CLOSE)

	return &ast.BlockStatement{
		CurlyBracketOpen:  pos,
		List:              stmts,
		CurlyBracketClose: end,
	}
}

func (p *Parser) continueAssignmentStatement(name *ast.Identifier) *ast.AssignmentStatement {
	p.expect(token.IS)
	value := p.parseExpression()
	end := p.expect(token.SEMICOLON)

	return &ast.AssignmentStatement{
		Name:      name,
		Value:     value,
		Semicolon: end,
	}
}

func (p *Parser) continueFunctionCallStatement(name *ast.Identifier) *ast.FunctionCallStatement {
	call := p.continueFunctionCallExpression(name)

	end := p.expect(token.SEMICOLON)

	return &ast.FunctionCallStatement{
		FunctionCall: call,
		Semicolon:    end,
	}
}
