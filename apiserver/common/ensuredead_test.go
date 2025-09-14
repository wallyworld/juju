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
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type deadEnsurerSuite struct{}

func TestDeadEnsurerSuite(t *tctesting.T) {
	tc.Run(t, &deadEnsurerSuite{})
}

type fakeDeadEnsurer struct {
	state.Entity
	life state.Life
	err  error
	fetchError
}

func (e *fakeDeadEnsurer) EnsureDead() error {
	return e.err
}

func (e *fakeDeadEnsurer) Life() state.Life {
	return e.life
}

func (*deadEnsurerSuite) TestEnsureDead(c *tc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeDeadEnsurer{life: state.Dying, err: fmt.Errorf("x0 fails")},
			u("x/1"): &fakeDeadEnsurer{life: state.Alive},
			u("x/2"): &fakeDeadEnsurer{life: state.Dying},
			u("x/3"): &fakeDeadEnsurer{life: state.Dead},
			u("x/4"): &fakeDeadEnsurer{fetchError: "x4 error"},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x1 := u("x/1")
		x2 := u("x/2")
		x4 := u("x/4")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1 || tag == x2 || tag == x4
		}, nil
	}
	afterDeadCalled := false
	afterDead := func(tag names.Tag) {
		if tag != u("x/1") && tag != u("x/2") {
			c.Fail()
		}
		afterDeadCalled = true
	}

	d := common.NewDeadEnsurer(st, afterDead, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"}, {"unit-x-4"}, {"unit-x-5"},
	}}
	result, err := d.EnsureDead(entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(afterDeadCalled, tc.IsTrue)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: "x0 fails"}},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: "x4 error"}},
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (*deadEnsurerSuite) TestEnsureDeadError(c *tc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	d := common.NewDeadEnsurer(&fakeState{}, nil, getCanModify)
	_, err := d.EnsureDead(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (*removeSuite) TestEnsureDeadNoArgsNoError(c *tc.C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	d := common.NewDeadEnsurer(&fakeState{}, nil, getCanModify)
	result, err := d.EnsureDead(params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}
