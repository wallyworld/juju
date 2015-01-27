// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/storage"
)

// stateStepsFor123 returns upgrade steps form Juju 1.23 that manipulate state directly.
func stateStepsFor123() []Step {
	// TODO(axw) stop checking feature flag once storage has graduated.
	if featureflag.Enabled(storage.FeatureFlag) {
		return []Step{
			// TODO - move to api steps once api is available
			&upgradeStep{
				description: "add default storage pools",
				targets:     []Target{DatabaseMaster},
				run: func(context Context) error {
					return addDefaultStoragePools(context.State())
				},
			},
		}
	}
	return []Step{}
}

// stepsFor123 returns upgrade steps form Juju 1.23 that only need the API.
func stepsFor123() []Step {
	return []Step{
		&upgradeStep{
			description: "add environment UUID to agent config",
			targets:     []Target{AllMachines},
			run:         addEnvironmentUUIDToAgentConfig,
		},
	}
}
