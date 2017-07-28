// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// Client provides access to the crossmodelrelations api facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client-side CrossModelRelations facade.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "CrossModelRelations")
	return &Client{ClientFacade: frontend, facade: backend}
}

func (c *Client) Close() error {
	return c.ClientFacade.Close()
}

// PublishRelationChange publishes relation changes to the
// model hosting the remote application involved in the relation.
func (c *Client) PublishRelationChange(change params.RemoteRelationChangeEvent) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("PublishRelationChanges", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	err = results.OneError()
	if err != nil {
		if params.IsCodeNotFound(err) {
			return errors.NotFoundf("relation for event %v", change)
		}
	}
	return err
}

func (c *Client) PublishIngressNetworkChange(change params.IngressNetworksChangeEvent) error {
	args := params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("PublishIngressNetworkChanges", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// RegisterRemoteRelations sets up the remote model to participate
// in the specified relations.
func (c *Client) RegisterRemoteRelations(relations ...params.RegisterRemoteRelationArg) ([]params.RegisterRemoteRelationResult, error) {
	args := params.RegisterRemoteRelationArgs{Relations: relations}
	var results params.RegisterRemoteRelationResults
	err := c.facade.FacadeCall("RegisterRemoteRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(relations) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(relations), len(results.Results))
	}
	return results.Results, nil
}

// WatchRelationUnits returns a watcher that notifies of changes to the
// units in the remote model for the relation with the given remote token.
func (c *Client) WatchRelationUnits(remoteRelationArg params.RemoteEntityArg) (watcher.RelationUnitsWatcher, error) {
	args := params.RemoteEntityArgs{Args: []params.RemoteEntityArg{remoteRelationArg}}
	var results params.RelationUnitsWatchResults
	err := c.facade.FacadeCall("WatchRelationUnits", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewRelationUnitsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// RelationUnitSettings returns the relation unit settings for the given relation units in the remote model.
func (c *Client) RelationUnitSettings(relationUnits []params.RemoteRelationUnit) ([]params.SettingsResult, error) {
	args := params.RemoteRelationUnits{relationUnits}
	var results params.SettingsResults
	err := c.facade.FacadeCall("RelationUnitSettings", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(relationUnits) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(relationUnits), len(results.Results))
	}
	return results.Results, nil
}

// WatchEgressAddressesForRelation returns a watcher that notifies when addresses,
// from which connections will originate to the offering side of the relation, change.
// Each event contains the entire set of addresses which the offering side is required
// to allow for access to the other side of the relation.
func (c *Client) WatchEgressAddressesForRelation(remoteRelationArg params.RemoteEntityArg) (watcher.StringsWatcher, error) {
	args := params.RemoteEntityArgs{Args: []params.RemoteEntityArg{remoteRelationArg}}
	var results params.StringsWatchResults
	err := c.facade.FacadeCall("WatchEgressAddressesForRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchRelationStatus starts a RelationStatusWatcher for watching the life and
// status of the specified relation in the remote model.
func (c *Client) WatchRelationStatus(arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
	args := params.RemoteEntityArgs{Args: []params.RemoteEntityArg{arg}}
	var results params.RelationStatusWatchResults
	err := c.facade.FacadeCall("WatchRelationsStatus", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewRelationStatusWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
