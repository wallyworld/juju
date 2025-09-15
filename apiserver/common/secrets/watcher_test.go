// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	tctesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	testhelpers.IsolationSuite
}

func TestWatcherSuite(t *tctesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestSecretBackendModelConfigWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelConfigChangesWatcher := mocks.NewMockNotifyWatcher(ctrl)
	model := mocks.NewMockModel(ctrl)

	ch := make(chan struct{}, 3)
	done := make(chan struct{})
	receiverReady := make(chan struct{})
	defer close(receiverReady)
	go func() {
		for {
			_, ok := <-receiverReady
			if !ok {
				return
			}
			ch <- struct{}{}
		}
	}()
	receiverReady <- struct{}{}

	modelConfigChangesWatcher.EXPECT().Wait().Return(nil)
	modelConfigChangesWatcher.EXPECT().Changes().Return(ch).AnyTimes()

	gomock.InOrder(
		model.EXPECT().ModelConfig().DoAndReturn(
			// Initail call to get the current secret backend.
			func() (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, tc.ErrorIsNil)
				return cfg, nil
			},
		),
		model.EXPECT().ModelConfig().DoAndReturn(
			// Call to get the current secret backend after the first change(no change, but we always send the initial event).
			func() (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, tc.ErrorIsNil)
				return cfg, nil
			},
		),
		model.EXPECT().ModelConfig().DoAndReturn(
			// Call to get the current secret backend after the first change(no change, we won't send the event).
			func() (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, tc.ErrorIsNil)
				return cfg, nil
			},
		),
		model.EXPECT().ModelConfig().DoAndReturn(
			// Call to get the current secret backend after the second change - backend changed.
			func() (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "a-different-backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, tc.ErrorIsNil)
				close(done)
				return cfg, nil
			},
		),
	)

	w, err := secrets.NewSecretBackendModelConfigWatcher(model, modelConfigChangesWatcher)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })

	received := 0
ensureReceived:
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		select {
		case _, ok := <-w.Changes():
			if !ok {
				break ensureReceived
			}
			received++
		case <-time.After(coretesting.ShortWait):
		}

		if received == 2 {
			return
		}

		select {
		case receiverReady <- struct{}{}:
		case <-done:
			break ensureReceived
		case <-time.After(coretesting.ShortWait):
		}

	}
	c.Fatalf("expected 2 events, got %d", received)
}
