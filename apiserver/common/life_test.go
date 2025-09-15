// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type lifeSuite struct{}

func TestLifeSuite(t *tctesting.T) {
	tc.Run(t, &lifeSuite{})
}

type fakeLifer struct {
	state.Entity
	life state.Life
	fetchError
}

func (l *fakeLifer) Life() state.Life {
	return l.life
}

func (*lifeSuite) TestLife(c *tc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeLifer{life: state.Alive},
			u("x/1"): &fakeLifer{life: state.Dying},
			u("x/2"): &fakeLifer{life: state.Dead},
			u("x/3"): &fakeLifer{fetchError: "x3 error"},
		},
	}
	getCanRead := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x2 := u("x/2")
		x3 := u("x/3")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x2 || tag == x3
		}, nil
	}
	lg := common.NewLifeGetter(st, getCanRead)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"},
	}}
	results, err := lg.Life(entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: life.Dead},
			{Error: &params.Error{Message: "x3 error"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*lifeSuite) TestLifeError(c *tc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewLifeGetter(&fakeState{}, getCanRead)
	_, err := lg.Life(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (*lifeSuite) TestLifeNoArgsNoError(c *tc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewLifeGetter(&fakeState{}, getCanRead)
	result, err := lg.Life(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}
