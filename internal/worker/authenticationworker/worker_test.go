// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	"strings"
	tctesting "testing"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3/ssh"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/keyupdater"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/authenticationworker"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type workerSuite struct {
	jujutesting.JujuConnSuite
	machine       *state.Machine
	keyupdaterAPI *keyupdater.State

	existingEnvKey string
	existingKeys   []string
}

func TestWorkerSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// Default ssh user is currently "ubuntu".
	c.Assert(authenticationworker.SSHUser, tc.Equals, "ubuntu")
	// Set the ssh user to empty (the current user) as required by the test infrastructure.
	s.PatchValue(&authenticationworker.SSHUser, "")

	// Replace the default dummy key in the test environment with a valid one.
	// This will be added to the ssh authorised keys when the agent starts.
	s.setAuthorisedKeys(c, sshtesting.ValidKeyOne.Key+" firstuser@host")
	// Record the existing key with its prefix for testing later.
	s.existingEnvKey = sshtesting.ValidKeyOne.Key + " Juju:firstuser@host"

	// Set up an existing key (which is not in the environment) in the ssh authorised_keys file.
	s.existingKeys = []string{sshtesting.ValidKeyTwo.Key + " existinguser@host"}
	err := ssh.AddKeys(authenticationworker.SSHUser, s.existingKeys...)
	c.Assert(err, tc.ErrorIsNil)

	var apiRoot api.Connection
	apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	c.Assert(apiRoot, tc.NotNil)
	s.keyupdaterAPI = keyupdater.NewState(apiRoot)
	c.Assert(s.keyupdaterAPI, tc.NotNil)
}

func stop(c *tc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), tc.IsNil)
}

type mockConfig struct {
	agent.Config
	c   *tc.C
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func agentConfig(c *tc.C, tag names.MachineTag) *mockConfig {
	return &mockConfig{c: c, tag: tag}
}

func (s *workerSuite) setAuthorisedKeys(c *tc.C, keys ...string) {
	keyStr := strings.Join(keys, "\n")
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": keyStr}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *workerSuite) waitSSHKeys(c *tc.C, expected []string) {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authoirsed ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, tc.ErrorIsNil)
			keysStr := strings.Join(keys, "\n")
			expectedStr := strings.Join(expected, "\n")
			if expectedStr != keysStr {
				continue
			}
			return
		}
	}
}

func (s *workerSuite) TestKeyUpdateRetainsExisting(c *tc.C) {
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterAPI, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, authWorker)

	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)
	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestNewKeysInJujuAreSavedOnStartup(c *tc.C) {
	newKey := sshtesting.ValidKeyThree.Key + " user@host"
	s.setAuthorisedKeys(c, newKey)

	authWorker, err := authenticationworker.NewWorker(s.keyupdaterAPI, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, authWorker)

	newKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:user@host"
	s.waitSSHKeys(c, append(s.existingKeys, newKeyWithCommentPrefix))
}

func (s *workerSuite) TestDeleteKey(c *tc.C) {
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterAPI, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, authWorker)

	// Add another key
	anotherKey := sshtesting.ValidKeyThree.Key + " another@host"
	s.setAuthorisedKeys(c, s.existingEnvKey, anotherKey)
	anotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:another@host"
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey, anotherKeyWithCommentPrefix))

	// Delete the original key and check anotherKey plus the existing keys remain.
	s.setAuthorisedKeys(c, anotherKey)
	s.waitSSHKeys(c, append(s.existingKeys, anotherKeyWithCommentPrefix))
}

func (s *workerSuite) TestMultipleChanges(c *tc.C) {
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterAPI, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Perform a set to add a key and delete a key.
	// added: key 3
	// deleted: key 1 (existing env key)
	s.setAuthorisedKeys(c, sshtesting.ValidKeyThree.Key+" yetanother@host")
	yetAnotherKeyWithComment := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithComment))
}

func (s *workerSuite) TestWorkerRestart(c *tc.C) {
	authWorker, err := authenticationworker.NewWorker(s.keyupdaterAPI, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, authWorker)
	s.waitSSHKeys(c, append(s.existingKeys, s.existingEnvKey))

	// Stop the worker and delete and add keys from the environment while it is down.
	// added: key 3
	// deleted: key 1 (existing env key)
	stop(c, authWorker)
	s.setAuthorisedKeys(c, sshtesting.ValidKeyThree.Key+" yetanother@host")

	// Restart the worker and check that the ssh auth keys are as expected.
	authWorker, err = authenticationworker.NewWorker(s.keyupdaterAPI, agentConfig(c, s.machine.Tag().(names.MachineTag)))
	c.Assert(err, tc.ErrorIsNil)
	defer stop(c, authWorker)

	yetAnotherKeyWithCommentPrefix := sshtesting.ValidKeyThree.Key + " Juju:yetanother@host"
	s.waitSSHKeys(c, append(s.existingKeys, yetAnotherKeyWithCommentPrefix))
}
