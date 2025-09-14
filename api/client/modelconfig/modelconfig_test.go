// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

type modelconfigSuite struct{}

func TestModelconfigSuite(t *tctesting.T) {
	tc.Run(t, &modelconfigSuite{})
}

func (s *modelconfigSuite) TestModelGet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelConfigResults)
	results := params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"foo": {"bar", "model"},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelGet", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ModelGet()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}

func (s *modelconfigSuite) TestModelGetWithMetadata(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelConfigResults)
	results := params.ModelConfigResults{
		Config: map[string]params.ConfigValue{
			"foo": {"bar", "model"},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelGet", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ModelGetWithMetadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, config.ConfigValues{
		"foo": {"bar", "model"},
	})
}

func (s *modelconfigSuite) TestModelSet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelSet{
		Config: map[string]interface{}{
			"some-name":  "value",
			"other-name": true,
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelSet", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.ModelSet(map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelconfigSuite) TestModelUnset(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelUnset{
		Keys: []string{"foo", "bar"},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ModelUnset", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.ModelUnset("foo", "bar")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelconfigSuite) TestSetSupport(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.ModelSLA{
		ModelSLAInfo: params.ModelSLAInfo{
			Level: "foobar",
			Owner: "bob",
		},
		Credentials: []byte("creds"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetSLALevel", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetSLALevel("foobar", "bob", []byte("creds"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelconfigSuite) TestGetSupport(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.StringResult)
	results := params.StringResult{
		Result: "level",
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SLALevel", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	level, err := client.SLALevel()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(level, tc.Equals, "level")
}

func (s *modelconfigSuite) TestSequences(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.ModelSequencesResult)
	results := params.ModelSequencesResult{
		Sequences: map[string]int{"foo": 5, "bar": 2},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Sequences", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	sequences, err := client.Sequences()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sequences, tc.DeepEquals, map[string]int{"foo": 5, "bar": 2})
}

func (s *modelconfigSuite) TestGetModelConstraints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var args interface{}
	res := new(params.GetConstraintsResults)
	results := params.GetConstraintsResults{
		Constraints: constraints.MustParse("arch=amd64"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetModelConstraints", args, res).SetArg(2, results).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetModelConstraints()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, constraints.MustParse("arch=amd64"))
}

func (s *modelconfigSuite) TestSetModelConstraints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var res interface{}
	args := params.SetConstraints{
		Constraints: constraints.MustParse("arch=amd64"),
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("SetModelConstraints", args, res).Return(nil)
	client := modelconfig.NewClientFromCaller(mockFacadeCaller)
	err := client.SetModelConstraints(constraints.MustParse("arch=amd64"))
	c.Assert(err, tc.ErrorIsNil)
}
