// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"go.uber.org/mock/gomock"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/observer/metricobserver/mocks"
)

func createMockMetrics(c *tc.C, labels interface{}) (*mocks.MockMetricsCollector, func()) {
	ctrl := gomock.NewController(c)

	summary := mocks.NewMockSummary(ctrl)
	summary.EXPECT().Observe(gomock.Any()).AnyTimes()

	summaryVec := mocks.NewMockSummaryVec(ctrl)
	summaryVec.EXPECT().With(labels).Return(summary).AnyTimes()

	metricsCollector := mocks.NewMockMetricsCollector(ctrl)
	metricsCollector.EXPECT().APIRequestDuration().Return(summaryVec).AnyTimes()

	return metricsCollector, ctrl.Finish
}
