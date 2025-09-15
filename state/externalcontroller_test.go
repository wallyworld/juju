// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type externalControllerSuite struct {
	ConnSuite
	externalControllers state.ExternalControllers
}

func TestExternalControllerSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &externalControllerSuite{})
}

func (s *externalControllerSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.externalControllers = state.NewExternalControllers(s.State)
}

func (s *externalControllerSuite) TestSaveInvalidAddress(c *tc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0"},
		CACert:        testing.CACert,
	}
	_, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`controller api address "192.168.1.0" not valid`))
}

func (s *externalControllerSuite) TestSaveNoModels(c *tc.C) {
	controllerInfo := defaultControllerInfo()
	ec, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ec.Id(), tc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), tc.DeepEquals, controllerInfo)
	s.assertSavedControllerInfo(c, controllerInfo)
}

func (s *externalControllerSuite) TestSave(c *tc.C) {
	controllerInfo := defaultControllerInfo()
	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	ec, err := s.externalControllers.Save(controllerInfo, uuid1, uuid2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ec.Id(), tc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), tc.DeepEquals, controllerInfo)
	s.assertSavedControllerInfo(c, controllerInfo, uuid1, uuid2)
}

func (s *externalControllerSuite) TestSaveIdempotent(c *tc.C) {
	controllerInfo := defaultControllerInfo()
	uuid1 := utils.MustNewUUID().String()
	_, err := s.externalControllers.Save(controllerInfo, uuid1)
	c.Assert(err, tc.ErrorIsNil)
	ec, err := s.externalControllers.Save(controllerInfo, uuid1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ec.Id(), tc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), tc.DeepEquals, controllerInfo)
	s.assertSavedControllerInfo(c, controllerInfo, uuid1)
}

func (s *externalControllerSuite) TestUpdateModels(c *tc.C) {
	controllerInfo := defaultControllerInfo()
	uuid1 := utils.MustNewUUID().String()
	_, err := s.externalControllers.Save(controllerInfo, uuid1)
	c.Assert(err, tc.ErrorIsNil)
	uuid2 := utils.MustNewUUID().String()
	_, err = s.externalControllers.Save(controllerInfo, uuid2)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedControllerInfo(c, controllerInfo, uuid1, uuid2)
}

func (s *externalControllerSuite) TestSaveAndMoveModels(c *tc.C) {
	// Add a new controller associated with 2 models.
	oldController := defaultControllerInfo()
	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	_, err := s.externalControllers.Save(oldController, uuid1, uuid2)
	c.Assert(err, tc.ErrorIsNil)
	s.assertSavedControllerInfo(c, oldController, uuid1, uuid2)

	// Now add a second controller associated with 2 models,
	// one of which is a model against the old controller.
	newController := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
		Alias:         "another-alias",
		Addrs:         []string{"192.168.2.0:1234", "10.0.2.1:1234"},
		CACert:        "any-old-cert",
	}

	uuid3 := utils.MustNewUUID().String()
	err = s.externalControllers.SaveAndMoveModels(newController, uuid2, uuid3)
	c.Assert(err, tc.ErrorIsNil)

	// New controller is created and associated with models.
	s.assertSavedControllerInfo(c, newController, uuid2, uuid3)

	// Old controller is no longer associated with the 2nd model UUID.
	s.assertSavedControllerInfo(c, oldController, uuid1)
}

func (s *externalControllerSuite) TestControllerForModel(c *tc.C) {
	controllerInfo := defaultControllerInfo()
	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	ec, err := s.externalControllers.Save(controllerInfo, uuid1, uuid2)
	c.Assert(err, tc.ErrorIsNil)
	found, err := s.externalControllers.ControllerForModel(uuid1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ec, tc.DeepEquals, found)
	_, err = s.externalControllers.ControllerForModel("1234")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *externalControllerSuite) TestController(c *tc.C) {
	controllerInfo := defaultControllerInfo()
	_, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)

	ec, err := s.externalControllers.Controller(testing.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ec, tc.NotNil)
	c.Assert(ec.Id(), tc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), tc.DeepEquals, controllerInfo)
}

func (s *externalControllerSuite) TestControllerNotFound(c *tc.C) {
	ec, err := s.externalControllers.Controller("foo")
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	c.Assert(err, tc.ErrorMatches, `external controller with UUID foo not found`)
	c.Assert(ec, tc.IsNil)
}

func (s *externalControllerSuite) TestWatchController(c *tc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Alias:         "alias1",
		Addrs:         []string{"192.168.1.0:1234"},
		CACert:        testing.CACert,
	}
	_, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)

	w := s.externalControllers.WatchController(testing.ControllerTag.Id())
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Update the alias, check for one change.
	controllerInfo.Alias = "alias2"
	_, err = s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()

	// Update the alias and addresses, check for one change.
	controllerInfo.Alias = "alias3"
	controllerInfo.Addrs = []string{"192.168.1.1:1234"}
	_, err = s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *externalControllerSuite) TestWatch(c *tc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Alias:         "alias1",
		Addrs:         []string{"192.168.1.0:1234"},
		CACert:        testing.CACert,
	}
	_, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)

	w := s.externalControllers.Watch()
	defer statetesting.AssertStop(c, w)

	// Initial event.
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange(testing.ControllerTag.Id())
	wc.AssertNoChange()

	// Update the controller, expect no change. We only get
	// updated on addition and removal.
	controllerInfo.Alias = "alias2"
	_, err = s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the controller, we should get a change.
	err = s.externalControllers.Remove(testing.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(testing.ControllerTag.Id())
	wc.AssertNoChange()

	// Removing a non-existent controller shouldn't trigger
	// a change.
	err = s.externalControllers.Remove("fnord")
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertNoChange()

	// Add the controller again, and we should see a change.
	_, err = s.externalControllers.Save(controllerInfo)
	c.Assert(err, tc.ErrorIsNil)
	wc.AssertChange(testing.ControllerTag.Id())
	wc.AssertNoChange()
}

func (s *externalControllerSuite) assertSavedControllerInfo(
	c *tc.C, controller crossmodel.ControllerInfo, modelUUIDs ...string,
) {
	coll, closer := state.GetCollection(s.State, "externalControllers")
	defer closer()

	var raw bson.M
	err := coll.FindId(controller.ControllerTag.Id()).One(&raw)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(raw["_id"], tc.Equals, controller.ControllerTag.Id())
	c.Assert(raw["cacert"], tc.Equals, controller.CACert)
	c.Assert(raw["alias"], tc.Equals, controller.Alias)

	var addresses []string
	for _, addr := range raw["addresses"].([]interface{}) {
		addresses = append(addresses, addr.(string))
	}
	c.Assert(addresses, tc.SameContents, controller.Addrs)

	var models []string
	for _, m := range raw["models"].([]interface{}) {
		models = append(models, m.(string))
	}
	c.Assert(models, tc.SameContents, modelUUIDs)
}

func defaultControllerInfo() crossmodel.ControllerInfo {
	return crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Alias:         "controller-alias",
		Addrs:         []string{"192.168.1.0:1234", "10.0.0.1:1234"},
		CACert:        testing.CACert,
	}
}
