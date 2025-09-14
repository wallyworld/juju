// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"bufio"
	"bytes"
	"io"
	"os"
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

type querySuite struct{}

func TestQuerySuite(t *tctesting.T) {
	tc.Run(t, &querySuite{})
}

func (s *querySuite) TestSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	funcScope.EXPECT().Call(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()

	scope := NewMockScope(ctrl)

	res, err := os.ReadFile("./testfiles/success")
	c.Assert(err, tc.ErrorIsNil)

	buf := bufio.NewReader(bytes.NewBuffer(res))
	for {
		line, _, err := buf.ReadLine()
		if err == io.EOF {
			break
		}

		c.Logf("Line: %v", string(line))

		query, err := Parse(string(line))
		c.Assert(err, tc.ErrorIsNil)

		done, err := query.Run(funcScope, scope)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(done, tc.IsTrue)
	}
}

func (s *querySuite) TestFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	funcScope.EXPECT().Call(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

	scope := NewMockScope(ctrl)

	res, err := os.ReadFile("./testfiles/failure")
	c.Assert(err, tc.ErrorIsNil)

	buf := bufio.NewReader(bytes.NewBuffer(res))
	for {
		line, _, err := buf.ReadLine()
		if err == io.EOF {
			break
		}

		c.Logf("Line: %v", string(line))

		query, err := Parse(string(line))
		c.Assert(err, tc.ErrorIsNil)

		done, err := query.Run(funcScope, scope)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(done, tc.IsFalse)
	}
}

func (s *querySuite) TestQueryScope(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)

	scope := NewMockScope(ctrl)
	scope.EXPECT().GetIdentValue("life").Return(&BoxString{value: "alive"}, nil).Times(2)

	src := `life == "death" || life == "alive"`

	query, err := Parse(src)
	c.Assert(err, tc.ErrorIsNil)

	done, err := query.Run(funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(done, tc.IsTrue)
}

func (s *querySuite) TestRunIdent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)

	scope := NewMockScope(ctrl)
	scope.EXPECT().GetIdentValue("life").Return(NewString("alive"), nil)

	exp := &QueryExpression{
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
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, NewString("alive"))
}

func (s *querySuite) TestRunString(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &String{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    STRING,
						Literal: "abc",
					},
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    STRING,
					Literal: "abc",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &BoxString{"abc"})
}

func (s *querySuite) TestRunInteger(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
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
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &BoxInteger{value: int64(1)})
}

func (s *querySuite) TestRunFloat(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
		Expressions: []Expression{
			&ExpressionStatement{
				Expression: &Float{
					Token: Token{
						Pos:     Position{Line: 1, Column: 1, Offset: 0},
						Type:    FLOAT,
						Literal: "1.12",
					},
					Value: 1.12,
				},
				Token: Token{
					Pos:     Position{Line: 1, Column: 1, Offset: 0},
					Type:    FLOAT,
					Literal: "1.12",
				},
			},
		},
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &BoxFloat{value: float64(1.12)})
}

func (s *querySuite) TestRunBool(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
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
		},
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &BoxBool{value: true})
}

func (s *querySuite) TestRunInfixLogicalAND(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
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
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, true)
}

func (s *querySuite) TestRunInfixLogicalOR(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	funcScope := NewMockFuncScope(ctrl)
	scope := NewMockScope(ctrl)

	exp := &QueryExpression{
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
							Literal: "false",
						},
						Value: false,
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
	}

	var query Query
	result, err := query.run(exp, funcScope, scope)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, true)
}
