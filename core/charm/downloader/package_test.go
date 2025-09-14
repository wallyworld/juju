// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/logger_mocks.go github.com/juju/juju/core/charm/downloader Logger
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/charm_mocks.go github.com/juju/juju/core/charm/downloader CharmArchive,CharmRepository
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/storage_mocks.go github.com/juju/juju/core/charm/downloader RepositoryGetter,Storage
