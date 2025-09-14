// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/juju/loggo"
)

//go:generate go run go.uber.org/mock/mockgen -package database -destination network_mock_test.go github.com/juju/juju/core/network ConfigSource,ConfigSourceNIC,ConfigSourceAddr

type stubLogger struct{}

func (stubLogger) Errorf(string, ...interface{})            {}
func (stubLogger) Warningf(string, ...interface{})          {}
func (stubLogger) Debugf(string, ...interface{})            {}
func (stubLogger) Logf(loggo.Level, string, ...interface{}) {}
