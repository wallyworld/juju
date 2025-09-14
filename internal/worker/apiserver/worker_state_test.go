// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	tctesting "testing"

	"github.com/juju/collections/set"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"

	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication/jwt"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/internal/jwtparser"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiserver"
	statetesting "github.com/juju/juju/state/testing"
)

type WorkerStateSuite struct {
	workerFixture
	statetesting.StateSuite
}

func TestWorkerStateSuite(t *tctesting.T) {
	tc.Run(t, &WorkerStateSuite{})
}

func (s *WorkerStateSuite) SetUpSuite(c *tc.C) {
	s.workerFixture.SetUpSuite(c)

	mgotesting.MgoServer.EnableReplicaSet = true
	err := mgotesting.MgoServer.Start(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.workerFixture.AddCleanup(func(*tc.C) { mgotesting.MgoServer.Destroy() })

	s.StateSuite.SetUpSuite(c)
}

func (s *WorkerStateSuite) TearDownSuite(c *tc.C) {
	s.StateSuite.TearDownSuite(c)
	s.workerFixture.TearDownSuite(c)
}

func (s *WorkerStateSuite) SetUpTest(c *tc.C) {
	s.workerFixture.SetUpTest(c)
	s.StateSuite.SetUpTest(c)
	s.config.StatePool = s.StatePool
	s.config.GetAuditConfig = func() auditlog.Config {
		return auditlog.Config{
			Enabled:        true,
			CaptureAPIArgs: true,
			MaxSizeMB:      200,
			MaxBackups:     5,
			ExcludeMethods: set.NewStrings("Exclude.This"),
			Target:         &apitesting.FakeAuditLog{},
		}
	}
}

func (s *WorkerStateSuite) TearDownTest(c *tc.C) {
	s.StateSuite.TearDownTest(c)
	s.workerFixture.TearDownTest(c)
}

func (s *WorkerStateSuite) TestStart(c *tc.C) {
	w, err := apiserver.NewWorker(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// The server is started some time after the worker
	// starts, not necessarily as soon as NewWorker returns.
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.stub.Calls()) == 0 {
			continue
		}
		break
	}
	if !s.stub.CheckCallNames(c, "NewServer") {
		return
	}
	args := s.stub.Calls()[0].Args
	c.Assert(args, tc.HasLen, 1)
	c.Assert(args[0], tc.FitsTypeOf, coreapiserver.ServerConfig{})
	config := args[0].(coreapiserver.ServerConfig)

	c.Assert(config.RegisterIntrospectionHandlers, tc.NotNil)
	config.RegisterIntrospectionHandlers = nil

	c.Assert(config.UpgradeComplete, tc.NotNil)
	config.UpgradeComplete = nil

	c.Assert(config.NewObserver, tc.NotNil)
	config.NewObserver = nil

	c.Assert(config.GetAuditConfig, tc.NotNil)
	// Set the audit config getter to Nil because we don't want to
	// compare it.
	config.GetAuditConfig = nil

	c.Assert(config.Presence, tc.NotNil)
	config.Presence = nil

	logSinkConfig := coreapiserver.DefaultLogSinkConfig()

	jwtAuthenticator := jwt.NewAuthenticator(&jwtparser.Parser{})

	c.Assert(config, tc.DeepEquals, coreapiserver.ServerConfig{
		StatePool:                  s.StatePool,
		LocalMacaroonAuthenticator: s.authenticator,
		Mux:                        s.mux,
		Clock:                      s.clock,
		Controller:                 s.controller,
		MultiwatcherFactory:        s.multiwatcherFactory,
		Tag:                        s.agentConfig.Tag(),
		DataDir:                    s.agentConfig.DataDir(),
		LogDir:                     s.agentConfig.LogDir(),
		Hub:                        &s.hub,
		PublicDNSName:              "",
		AllowModelAccess:           false,
		LogSinkConfig:              &logSinkConfig,
		LeaseManager:               s.leaseManager,
		MetricsCollector:           s.metricsCollector,
		SysLogger:                  s.sysLogger,
		CharmhubHTTPClient:         s.charmhubHTTPClient,
		DBGetter:                   s.dbGetter,
		JWTAuthenticator:           jwtAuthenticator,
	})
}
