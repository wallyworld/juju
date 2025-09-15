// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	tctesting "testing"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/controller"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// legacySuite has the tests for the controller client-side facade
// which use JujuConnSuite. The plan is to gradually move these tests
// to Suite in controller_test.go.
type legacySuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper
}

func TestLegacySuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &legacySuite{})
}

func (s *legacySuite) OpenAPI(c *tc.C) *controller.Client {
	return controller.NewClient(s.OpenControllerAPI(c))
}

func (s *legacySuite) TestAllModels(c *tc.C) {
	owner := names.NewUserTag("user@remote")
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner}).Close()
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: owner}).Close()

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	models, err := sysManager.AllModels()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.HasLen, 3)

	var obtained []string
	for _, m := range models {
		c.Assert(m.Type, tc.Equals, model.IAAS)
		obtained = append(obtained, fmt.Sprintf("%s/%s", m.Owner, m.Name))
	}
	expected := []string{
		"admin/controller",
		"user@remote/first",
		"user@remote/second",
	}
	c.Assert(obtained, tc.SameContents, expected)
}

func (s *legacySuite) TestControllerConfig(c *tc.C) {
	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	cfg, err := sysManager.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	cfgFromDB, err := s.State.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg["controller-uuid"], tc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(int(cfg["state-port"].(float64)), tc.Equals, cfgFromDB.StatePort())
	c.Assert(int(cfg["api-port"].(float64)), tc.Equals, cfgFromDB.APIPort())
}

func (s *legacySuite) TestDestroyController(c *tc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{Name: "foo"})
	factory.NewFactory(st, s.StatePool).MakeMachine(c, nil) // make it non-empty
	st.Close()

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	err := sysManager.DestroyController(controller.DestroyControllerParams{})
	c.Assert(err, tc.ErrorMatches, `initiating destroy model operation: failed to destroy model: hosting 1 other model \(controller has hosted models\)`)
}

func (s *legacySuite) TestListBlockedModels(c *tc.C) {
	err := s.State.SwitchBlockOn(state.ChangeBlock, "change block for controller")
	c.Assert(err, tc.ErrorIsNil)
	err = s.State.SwitchBlockOn(state.DestroyBlock, "destroy block for controller")
	c.Assert(err, tc.ErrorIsNil)

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	results, err := sysManager.ListBlockedModels()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []params.ModelBlockInfo{
		{
			Name:     "controller",
			UUID:     s.State.ModelUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockChange",
				"BlockDestroy",
			},
		},
	})
}

func (s *legacySuite) TestRemoveBlocks(c *tc.C) {
	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	err := sysManager.RemoveBlocks()
	c.Assert(err, tc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocks, tc.HasLen, 0)
}

func (s *legacySuite) TestWatchAllModels(c *tc.C) {
	// The WatchAllModels infrastructure is comprehensively tested
	// else. This test just ensure that the API calls work end-to-end.
	sysManager := s.OpenAPI(c)
	defer sysManager.Close()

	w, err := sysManager.WatchAllModels()
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		err := w.Stop()
		c.Assert(err, tc.ErrorIsNil)
	}()

	deltasC := make(chan []params.Delta)
	go func() {
		deltas, err := w.Next()
		c.Assert(err, tc.ErrorIsNil)
		deltasC <- deltas
	}()

	select {
	case deltas := <-deltasC:
		c.Assert(deltas, tc.HasLen, 1)
		modelInfo := deltas[0].Entity.(*params.ModelUpdate)

		model, err := s.State.Model()
		c.Assert(err, tc.ErrorIsNil)
		cfg, err := model.Config()
		c.Assert(err, tc.ErrorIsNil)
		status, err := model.Status()
		c.Assert(err, tc.ErrorIsNil)

		// Resource tags are unmarshalled as map[string]interface{}
		// Convert to map[string]string for comparison.
		switch tags := modelInfo.Config[config.ResourceTagsKey].(type) {
		case map[string]interface{}:
			tagStrings := make(map[string]string)
			for tag, val := range tags {
				tagStrings[tag] = fmt.Sprintf("%v", val)
			}
			modelInfo.Config[config.ResourceTagsKey] = tagStrings
		}

		// Integer values are unmarshalled as float64 across
		// the (JSON) api boundary. Explicitly convert to int
		// for the following attributes.
		switch val := modelInfo.Config[config.NetBondReconfigureDelayKey].(type) {
		case float64:
			modelInfo.Config[config.NetBondReconfigureDelayKey] = int(val)
		}
		switch val := modelInfo.Config[config.NumProvisionWorkersKey].(type) {
		case float64:
			modelInfo.Config[config.NumProvisionWorkersKey] = int(val)
		}
		switch val := modelInfo.Config[config.NumContainerProvisionWorkersKey].(type) {
		case float64:
			modelInfo.Config[config.NumContainerProvisionWorkersKey] = int(val)
		}

		expectedStatus := params.StatusInfo{
			Current: status.Status,
			Message: status.Message,
		}
		modelInfo.Status.Since = nil
		c.Assert(modelInfo.ModelUUID, tc.Equals, model.UUID())
		c.Assert(modelInfo.Name, tc.Equals, model.Name())
		c.Assert(modelInfo.Life, tc.Equals, life.Alive)
		c.Assert(modelInfo.Owner, tc.Equals, model.Owner().Id())
		c.Assert(modelInfo.ControllerUUID, tc.Equals, model.ControllerUUID())
		c.Assert(modelInfo.Config, tc.DeepEquals, cfg.AllAttrs())
		c.Assert(modelInfo.Status, tc.DeepEquals, expectedStatus)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *legacySuite) TestAPIServerCanShutdownWithOutstandingNext(c *tc.C) {
	apiInfo := s.APIInfo(c)
	apiInfo.ModelTag = names.ModelTag{}
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	sysManager := controller.NewClient(apiState)
	defer sysManager.Close()

	w, err := sysManager.WatchAllModels()
	c.Assert(err, tc.ErrorIsNil)
	defer w.Stop()

	deltasC := make(chan struct{}, 2)
	go func() {
		defer close(deltasC)
		for {
			_, err := w.Next()
			if err != nil {
				return
			}
			deltasC <- struct{}{}
		}
	}()
	// Read the first event.
	select {
	case <-deltasC:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
	// Wait a little while for the Next call to actually arrive.
	time.Sleep(testing.ShortWait)

	// We should be able to close the server instance
	// even when there's an outstanding Next call.
	srvStopped := make(chan struct{})
	go func() {
		// Resetting the dummy environment will call Stop on the
		// embedded API server.
		start := time.Now()
		dummy.Reset(c)
		c.Logf("dummy.Reset() took %v", time.Since(start))
		close(srvStopped)
	}()

	select {
	case <-srvStopped:
	case <-time.After(time.Minute): // LongWait (10s) didn't seem quite long enough, see LP 1900931
		c.Fatal("timed out waiting for server to stop")
	}

	// Check that the Next call has returned too.
	select {
	case _, ok := <-deltasC:
		if ok {
			c.Fatalf("got unexpected event from deltasC")
		}
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *legacySuite) TestGetControllerAccess(c *tc.C) {
	controller := s.OpenAPI(c)
	defer controller.Close()
	err := controller.GrantController("fred@external", "superuser")
	c.Assert(err, tc.ErrorIsNil)
	access, err := controller.GetControllerAccess("fred@external")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.Access("superuser"))
}
