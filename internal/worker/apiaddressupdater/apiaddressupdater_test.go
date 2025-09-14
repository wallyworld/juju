// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/tc"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"

	apimachiner "github.com/juju/juju/api/agent/machiner"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiaddressupdater"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type APIAddressUpdaterSuite struct {
	jujutesting.JujuConnSuite
}

func TestAPIAddressUpdaterSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &APIAddressUpdaterSuite{})
}

func (s *APIAddressUpdaterSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, tc.ErrorIsNil)

	s.PatchValue(&network.AddressesForInterfaceName, func(string) ([]string, error) {
		return nil, nil
	})
}

type apiAddressSetter struct {
	servers chan []corenetwork.HostPorts
	err     error
}

func (s *apiAddressSetter) SetAPIHostPorts(servers []corenetwork.HostPorts) error {
	s.servers <- servers
	return s.err
}

func (s *APIAddressUpdaterSuite) TestStartStop(c *tc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    &apiAddressSetter{},
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, tc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), tc.IsNil)
}

func (s *APIAddressUpdaterSuite) TestAddressInitialUpdate(c *tc.C) {
	updatedServers := []corenetwork.SpaceHostPorts{corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1")}
	err := s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, tc.ErrorIsNil)

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	updater, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	expServer := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
	}.HostPorts()

	// SetAPIHostPorts should be called with the initial value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called")
	case servers := <-setter.servers:
		c.Assert(servers, tc.DeepEquals, []corenetwork.HostPorts{expServer})
	}

	// The values are also available through the report.
	reporter, ok := updater.(worker.Reporter)
	c.Assert(ok, tc.IsTrue)
	c.Assert(reporter.Report(), tc.DeepEquals, map[string]interface{}{
		"servers": [][]string{{"localhost:1234", "127.0.0.1:1234"}},
	})

}

func (s *APIAddressUpdaterSuite) TestAddressChange(c *tc.C) {
	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), tc.IsNil) }()
	defer worker.Kill()
	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
	}
	// SetAPIHostPorts should be called with the initial value (empty),
	// and then the updated value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, tc.HasLen, 0)
	}
	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, tc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func (s *APIAddressUpdaterSuite) TestAddressChangeEmpty(c *tc.C) {
	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), tc.IsNil) }()
	defer worker.Kill()

	// SetAPIHostPorts should be called with the initial value (empty),
	// and then the updated value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, tc.HasLen, 0)
	}

	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
	}

	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, tc.DeepEquals, []corenetwork.HostPorts{expServer})
	}

	updatedServers = []corenetwork.SpaceHostPorts{}
	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, tc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func (s *APIAddressUpdaterSuite) TestBridgeAddressesFiltering(c *tc.C) {
	s.PatchValue(&network.AddressesForInterfaceName, func(name string) ([]string, error) {
		if name == network.DefaultLXDBridge {
			return []string{
				"10.0.4.1",
				"10.0.4.4",
			}, nil
		} else if name == network.DefaultKVMBridge {
			return []string{
				"192.168.122.1",
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})

	initialServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
		corenetwork.NewSpaceHostPorts(
			4321,
			"10.0.3.3",      // not filtered
			"10.0.4.1",      // filtered lxd bridge address
			"10.0.4.2",      // not filtered
			"192.168.122.1", // filtered default virbr0
		),
	}
	err := s.State.SetAPIHostPorts(initialServers)
	c.Assert(err, tc.ErrorIsNil)

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	w, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Assert(w.Wait(), tc.IsNil) }()
	defer w.Kill()

	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
		corenetwork.NewSpaceHostPorts(
			4001,
			"10.0.3.3", // not filtered
		),
	}

	expServer1 := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
	}.HostPorts()

	// SetAPIHostPorts should be called with the initial value, and
	// then the updated value, but filtering occurs in both cases.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, tc.HasLen, 2)

		expServerInit := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.3.3").AsProviderAddress(), NetPort: 4321},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.4.2").AsProviderAddress(), NetPort: 4321},
		}.HostPorts()
		c.Assert(servers, tc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerInit})
	}

	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, tc.IsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		c.Assert(servers, tc.HasLen, 2)

		expServerUpd := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.3.3").AsProviderAddress(), NetPort: 4001},
		}.HostPorts()
		c.Assert(servers, tc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerUpd})
	}
}

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestValidateSuite(t *tctesting.T) {
	tc.Run(t, &ValidateSuite{})
}

func (*ValidateSuite) TestValid(c *tc.C) {
	err := validConfig().Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (*ValidateSuite) TestMissingAddresser(c *tc.C) {
	config := validConfig()
	config.Addresser = nil
	checkNotValid(c, config, "nil Addresser not valid")
}

func (*ValidateSuite) TestMissingSetter(c *tc.C) {
	config := validConfig()
	config.Setter = nil
	checkNotValid(c, config, "nil Setter not valid")
}

func (*ValidateSuite) TestMissingLogger(c *tc.C) {
	config := validConfig()
	config.Logger = nil
	checkNotValid(c, config, "nil Logger not valid")
}

func validConfig() apiaddressupdater.Config {
	return apiaddressupdater.Config{
		Addresser: struct{ apiaddressupdater.APIAddresser }{},
		Setter: struct {
			apiaddressupdater.APIAddressSetter
		}{},
		Logger: loggo.GetLogger("test"),
	}
}

func checkNotValid(c *tc.C, config apiaddressupdater.Config, expect string) {
	check := func(err error) {
		c.Check(err, tc.ErrorMatches, expect)
		c.Check(err, tc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := apiaddressupdater.NewAPIAddressUpdater(config)
	c.Check(worker, tc.IsNil)
	check(err)
}
