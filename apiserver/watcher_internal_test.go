// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	tctesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/mocks"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type allWatcherSuite struct {
	testing.BaseSuite
}

func TestAllWatcherSuite(t *tctesting.T) {
	tc.Run(t, &allWatcherSuite{})
}

func (s *allWatcherSuite) TestTranslateApplicationWithStatus(c *tc.C) {
	s.assertTranslateApplicationWithStatus(c, newAllWatcherDeltaTranslater())
}

func (s *allWatcherSuite) assertTranslateApplicationWithStatus(c *tc.C, t DeltaTranslater) {
	input := &multiwatcher.ApplicationInfo{
		ModelUUID: testing.ModelTag.Id(),
		Name:      "test-app",
		CharmURL:  "test-app",
		Life:      life.Alive,
		Status: multiwatcher.StatusInfo{
			Current: status.Active,
		},
	}
	output := t.TranslateApplication(input)
	c.Assert(output, tc.DeepEquals, &params.ApplicationInfo{
		ModelUUID: input.ModelUUID,
		Name:      input.Name,
		CharmURL:  input.CharmURL,
		Life:      input.Life,
		Status: params.StatusInfo{
			Current: status.Active,
		},
	})
}

func (s *allWatcherSuite) TestTranslateAction(c *tc.C) {
	t := newAllWatcherDeltaTranslater()
	input := &multiwatcher.ActionInfo{
		ModelUUID:  testing.ModelTag.Id(),
		ID:         "2",
		Parameters: map[string]interface{}{"foo": "bar"},
		Results:    map[string]interface{}{"done": true},
	}
	output := t.TranslateAction(input)
	c.Assert(output, tc.DeepEquals, &params.ActionInfo{
		ModelUUID: input.ModelUUID,
		Id:        input.ID,
		Receiver:  input.Receiver,
		Name:      input.Name,
		Status:    input.Status,
		Message:   input.Message,
		Enqueued:  input.Enqueued,
		Started:   input.Started,
		Completed: input.Completed,
	})
}

func newDelta(info multiwatcher.EntityInfo) multiwatcher.Delta {
	return multiwatcher.Delta{Entity: info}
}

func (s *allWatcherSuite) TestTranslate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	dt := mocks.NewMockDeltaTranslater(ctrl)

	gomock.InOrder(
		dt.EXPECT().TranslateModel(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateApplication(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateRemoteApplication(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateMachine(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateUnit(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateCharm(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateRelation(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateBranch(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateAnnotation(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateBlock(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateAction(gomock.Any()).Return(nil),
		dt.EXPECT().TranslateApplicationOffer(gomock.Any()).Return(nil),
	)

	deltas := []multiwatcher.Delta{
		newDelta(&multiwatcher.ModelInfo{}),
		newDelta(&multiwatcher.ApplicationInfo{}),
		newDelta(&multiwatcher.RemoteApplicationUpdate{}),
		newDelta(&multiwatcher.MachineInfo{}),
		newDelta(&multiwatcher.UnitInfo{}),
		newDelta(&multiwatcher.CharmInfo{}),
		newDelta(&multiwatcher.RelationInfo{}),
		newDelta(&multiwatcher.BranchInfo{}),
		newDelta(&multiwatcher.AnnotationInfo{}),
		newDelta(&multiwatcher.BlockInfo{}),
		newDelta(&multiwatcher.ActionInfo{}),
		newDelta(&multiwatcher.ApplicationOfferInfo{}),
	}
	_ = translate(dt, deltas)
}

func (s *allWatcherSuite) TestTranslateModelEmpty(c *tc.C) {
	translator := newAllWatcherDeltaTranslater()
	entityInfo := translator.TranslateModel(&multiwatcher.ModelInfo{
		Config: map[string]any{},
	})
	c.Assert(entityInfo, tc.NotNil)

	modelUpdate := entityInfo.(*params.ModelUpdate)
	c.Assert(modelUpdate, tc.NotNil)
}

func (s *allWatcherSuite) TestTranslateModelAgentVersion(c *tc.C) {
	current := coretesting.CurrentVersion()
	configAttrs := map[string]any{
		"name":                 "some-name",
		"type":                 "some-type",
		"uuid":                 coretesting.ModelTag.Id(),
		config.AgentVersionKey: current.Number.String(),
	}

	translator := newAllWatcherDeltaTranslater()
	entityInfo := translator.TranslateModel(&multiwatcher.ModelInfo{
		Config: configAttrs,
	})
	c.Assert(entityInfo, tc.NotNil)

	modelUpdate := entityInfo.(*params.ModelUpdate)
	c.Assert(modelUpdate, tc.NotNil)
	c.Assert(modelUpdate.Version, tc.Equals, current.Number.String())
}
