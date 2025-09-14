// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

//go:generate go run go.uber.org/mock/mockgen -package space -destination context_mock_test.go github.com/juju/juju/environs/context ProviderCallContext
//go:generate go run go.uber.org/mock/mockgen -package space -destination environs_mock_test.go github.com/juju/juju/environs BootstrapEnviron,NetworkingEnviron
//go:generate go run go.uber.org/mock/mockgen -package space -destination spaces_mock_test.go github.com/juju/juju/environs/space ReloadSpacesState,Space,Constraints
