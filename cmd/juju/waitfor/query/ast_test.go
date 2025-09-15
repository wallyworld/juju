// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	tctesting "testing"

	"github.com/juju/tc"
)

type astSuite struct{}

func TestAstSuite(t *tctesting.T) {
	tc.Run(t, &astSuite{})
}

func (p *astSuite) TestQueryExpressionString(c *tc.C) {
	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Literal: "abc",
					},
				},
			},
			&ExpressionStatement{
				Expression: &Identifier{
					Token: Token{
						Literal: "efg",
					},
				},
			},
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "abc;efg;")
}

func (p *astSuite) TestExpressionStatementEmptyString(c *tc.C) {
	exp := &ExpressionStatement{
		Expression: &Identifier{
			Token: Token{
				Literal: "",
			},
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, ";")
}

func (p *astSuite) TestExpressionStatementString(c *tc.C) {
	exp := &ExpressionStatement{
		Expression: &Identifier{
			Token: Token{
				Literal: "abc",
			},
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "abc;")
}

func (p *astSuite) TestInfixExpressionString(c *tc.C) {
	exp := &InfixExpression{
		Left: &Identifier{
			Token: Token{
				Literal: "abc",
			},
		},
		Operator: "&&",
		Right: &Identifier{
			Token: Token{
				Literal: "efg",
			},
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "(abc && efg)")
}

func (p *astSuite) TestIdentifierString(c *tc.C) {
	exp := &Identifier{
		Token: Token{
			Literal: "abc",
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "abc")
}

func (p *astSuite) TestEmptyString(c *tc.C) {
	exp := &Empty{
		Token: Token{
			Literal: "abc",
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "()")
}

func (p *astSuite) TestIntegerString(c *tc.C) {
	exp := &Integer{
		Token: Token{
			Literal: "1",
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "1")
}

func (p *astSuite) TestFloatString(c *tc.C) {
	exp := &Float{
		Token: Token{
			Literal: "1.123",
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "1.123")
}

func (p *astSuite) TestBoolString(c *tc.C) {
	exp := &Bool{
		Token: Token{
			Literal: "true",
		},
	}
	c.Assert(exp.String(), tc.DeepEquals, "true")
}
