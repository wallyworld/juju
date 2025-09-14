// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type statusSetter interface {
	SetStatus(status.StatusInfo) error
}

func primeStatusHistory(c *tc.C, clock clock.Clock, entity statusSetter,
	statusVal status.Status, count int, nextData func(int) map[string]interface{}, delta time.Duration, info string) {
	now := clock.Now().Add(-delta)
	for i := 0; i < count; i++ {
		c.Logf("setting status for %v", entity)
		data := nextData(i)
		t := now.Add(time.Duration(i) * time.Second)
		s := status.StatusInfo{
			Status:  statusVal,
			Message: info,
			Data:    data,
			Since:   &t,
		}
		err := entity.SetStatus(s)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func checkInitialWorkloadStatus(c *tc.C, statusInfo status.StatusInfo) {
	c.Check(statusInfo.Status, tc.Equals, status.Waiting)
	c.Check(statusInfo.Message, tc.Equals, "waiting for machine")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Check(statusInfo.Since, tc.NotNil)
}

func primeUnitStatusHistory(c *tc.C, clock clock.Clock, unit *state.Unit, count int, delta time.Duration) {
	primeStatusHistory(c, clock, unit, status.Active, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"$foo": i, "$delta": delta}
	}, delta, "")
}

func checkPrimedUnitStatus(c *tc.C, statusInfo status.StatusInfo, expect int, expectDelta time.Duration) {
	c.Check(statusInfo.Status, tc.Equals, status.Active)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{"$foo": expect, "$delta": int64(expectDelta)})
	c.Check(statusInfo.Since, tc.NotNil)
}

func checkInitialUnitAgentStatus(c *tc.C, statusInfo status.StatusInfo) {
	c.Check(statusInfo.Status, tc.Equals, status.Allocating)
	c.Check(statusInfo.Message, tc.Equals, "")
	c.Check(statusInfo.Data, tc.HasLen, 0)
	c.Assert(statusInfo.Since, tc.NotNil)
}

func primeUnitAgentStatusHistory(c *tc.C, clock clock.Clock, agent *state.UnitAgent, count int, delta time.Duration, info string) {
	primeStatusHistory(c, clock, agent, status.Executing, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"$bar": i, "$delta": delta}
	}, delta, info)
}

func checkPrimedUnitAgentStatus(c *tc.C, statusInfo status.StatusInfo, expect int, expectDelta time.Duration) {
	checkPrimedUnitAgentStatusWithCustomMessage(c, statusInfo, expect, expectDelta, "")
}

func checkPrimedUnitAgentStatusWithCustomMessage(c *tc.C, statusInfo status.StatusInfo, expect int, expectDelta time.Duration, info string) {
	c.Check(statusInfo.Message, tc.Equals, info)
	c.Check(statusInfo.Status, tc.Equals, status.Executing)
	c.Check(statusInfo.Data, tc.DeepEquals, map[string]interface{}{"$bar": expect, "$delta": int64(expectDelta)})
	c.Check(statusInfo.Since, tc.NotNil)
}

func checkPrimedUnitAgentStatusWithRegexMessage(c *tc.C, statusInfo status.StatusInfo, message *regexp.Regexp) {
	c.Check(message.MatchString(statusInfo.Message), tc.IsTrue)
	c.Check(statusInfo.Status, tc.Equals, status.Executing)
	c.Check(statusInfo.Since, tc.NotNil)
}
