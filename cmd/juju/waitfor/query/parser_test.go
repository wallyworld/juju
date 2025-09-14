// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	tctesting "testing"

	"github.com/juju/tc"
)

type parserSuite struct{}

func TestParserSuite(t *tctesting.T) {
	tc.Run(t, &parserSuite{})
}

func (p *parserSuite) TestParserMultipleExpressions(c *tc.C) {
	query := `life; abc;`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    IDENT,
						Literal: "life",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    IDENT,
					Literal: "life",
				},
			},
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 7, Offset: 6},
						Type:    IDENT,
						Literal: "abc",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 7, Offset: 6},
					Type:    IDENT,
					Literal: "abc",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserIdent(c *tc.C) {
	query := `life`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    IDENT,
						Literal: "life",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    IDENT,
					Literal: "life",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserString(c *tc.C) {
	query := `"abc"`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &String{
					Token: Token{
						Pos:        Position{Line: 1, Column: 1, Offset: 0},
						Type:       STRING,
						Literal:    "abc",
						Terminated: true,
					},
				},
				Token: Token{
					Pos:        Position{Line: 1, Column: 1, Offset: 0},
					Type:       STRING,
					Literal:    "abc",
					Terminated: true,
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInteger(c *tc.C) {
	query := `1`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Integer{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    INT,
						Literal: "1",
					},
					Value: 1,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    INT,
					Literal: "1",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserFloat(c *tc.C) {
	query := `1.1`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Float{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    FLOAT,
						Literal: "1.1",
					},
					Value: 1.1,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    FLOAT,
					Literal: "1.1",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserBool(c *tc.C) {
	query := `true false`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Bool{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    BOOL,
						Literal: "true",
					},
					Value: true,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    BOOL,
					Literal: "true",
				},
			},
			&ExpressionStatement{
				Expression: &Bool{
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    BOOL,
						Literal: "false",
					},
					Value: false,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 6, Offset: 5},
					Type:    BOOL,
					Literal: "false",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserGroup(c *tc.C) {
	query := `(abc)`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Pos:     Position{Line: 1, Column: 2, Offset: 1},
						Type:    IDENT,
						Literal: "abc",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    LPAREN,
					Literal: "(",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInfixLogicalAND(c *tc.C) {
	query := `true && true`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &InfixExpression{
					Left: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Operator: "&&",
					Right: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 9, Offset: 8},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    CONDAND,
						Literal: "&&",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    BOOL,
					Literal: "true",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInfixLogicalOR(c *tc.C) {
	query := `true || true`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &InfixExpression{
					Left: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Operator: "||",
					Right: &Bool{
						Token: Token{
							Pos:     Position{Line: 1, Column: 9, Offset: 8},
							Type:    BOOL,
							Literal: "true",
						},
						Value: true,
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 6, Offset: 5},
						Type:    CONDOR,
						Literal: "||",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    BOOL,
					Literal: "true",
				},
			},
		},
	})
}

func (p *parserSuite) TestParserInfixLambda(c *tc.C) {
	query := `_ => _`

	lex := NewLexer(query)
	parser := NewParser(lex)
	exp, err := parser.Run()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exp, tc.DeepEquals, &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &LambdaExpression{
					Argument: &Identifier{
						Token: Token{
							Pos:     Position{Line: 1, Column: 1, Offset: 0},
							Type:    UNDERSCORE,
							Literal: "_",
						},
					},
					Expressions: []Expression{
						&Identifier{
							Token: Token{
								Pos:     Position{Line: 1, Column: 6, Offset: 5},
								Type:    UNDERSCORE,
								Literal: "_",
							},
						},
					},
					Token: Token{
						Pos:     Position{Line: 1, Column: 3, Offset: 2},
						Type:    LAMBDA,
						Literal: "=>",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    UNDERSCORE,
					Literal: "_",
				},
			},
		},
	})
}
