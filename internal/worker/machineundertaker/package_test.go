// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

type fakeCredentialAPI struct{}

func (*fakeCredentialAPI) InvalidateModelCredential(reason string) error {
	return nil
}
