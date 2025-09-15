// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
)

type ModelSuite struct {
	testhelpers.IsolationSuite
}

func TestModelSuite(t *tctesting.T) {
	tc.Run(t, &ModelSuite{})
}

func (*ModelSuite) TestValidateBranchName(c *tc.C) {
	for _, t := range []struct {
		branchName string
		valid      bool
	}{
		{"", false},
		{model.GenerationMaster, false},
		{"something else", true},
	} {
		err := model.ValidateBranchName(t.branchName)
		if t.valid {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.Satisfies, errors.IsNotValid)
		}
	}
}
