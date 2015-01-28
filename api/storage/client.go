// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.storage")

// Client allows access to the storage API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the storage API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Storage")
	logger.Debugf("\nSTORAGE FRONT-END: %#v", frontend)
	logger.Debugf("\nSTORAGE BACK-END: %#v", backend)
	return &Client{ClientFacade: frontend, facade: backend}
}

// Show retrieves information about desired storage instances.
func (c *Client) Show(tags []names.StorageTag) ([]params.StorageInstance, error) {
	found := params.StorageShowResults{}
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag.String()}
	}
	if err := c.facade.FacadeCall("Show", params.Entities{Entities: entities}, &found); err != nil {
		return nil, errors.Trace(err)
	}
	all := []params.StorageInstance{}
	allErr := params.ErrorResults{}
	for _, result := range found.Results {
		if result.Error.Error != nil {
			allErr.Results = append(allErr.Results, result.Error)
			continue
		}
		all = append(all, result.Result)
	}
	return all, allErr.Combine()
}

// ListPools lists pools according to a given filter.
// If no filter was provided, this will return a list
// of all pools.
func (c *Client) ListPools(types, names []string) ([]params.StoragePool, error) {
	args := params.StoragePoolFilter{
		Names: names,
		Types: types,
	}
	found := params.StoragePoolsResult{}
	if err := c.facade.FacadeCall("ListPools", args, &found); err != nil {
		return nil, errors.Trace(err)
	}
	return found.Pools, nil
}

func (c *Client) List() ([]params.StorageInstance, error) {
	result := params.StorageListResult{}
	if err := c.facade.FacadeCall("List", nil, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return result.Instances, nil
}
