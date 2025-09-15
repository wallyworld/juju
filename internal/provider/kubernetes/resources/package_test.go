// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

type (
	Operation = operation
)

type ApplierForTest interface {
	Applier
	Operations() []Operation
}

func NewApplierForTest() ApplierForTest {
	return &applier{}
}

func (a *applier) Operations() []Operation {
	return a.ops
}
