package ast

import (
	"github.com/Minnozz/gompiler/token"
)

type Expression interface {
	astNode
}

type BadExpression struct {
}

type Identifier struct {
	Name string
}

type LiteralExpression struct {
	Kind  token.Token
	Value string
}

type BinaryExpression struct {
	Left     Expression
	Operator token.Token
	Right    Expression
}
