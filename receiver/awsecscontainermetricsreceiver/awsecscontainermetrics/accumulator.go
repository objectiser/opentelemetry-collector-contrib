// Copyright 2020, OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package awsecscontainermetrics

import (
	"time"

	"go.opentelemetry.io/collector/consumer/pdata"
)

// metricDataAccumulator defines the accumulator
type metricDataAccumulator struct {
	mds []pdata.Metrics
}

// getMetricsData generates OT Metrics data from task metadata and docker stats
func (acc *metricDataAccumulator) getMetricsData(containerStatsMap map[string]ContainerStats, metadata TaskMetadata) {

	taskMetrics := ECSMetrics{}
	timestamp := pdata.TimeToUnixNano(time.Now())
	taskResource := taskResource(metadata)

	for _, containerMetadata := range metadata.Containers {
		stats := containerStatsMap[containerMetadata.DockerID]
		containerMetrics := getContainerMetrics(stats)
		containerMetrics.MemoryReserved = *containerMetadata.Limits.Memory
		containerMetrics.CPUReserved = *containerMetadata.Limits.CPU

		if containerMetrics.CPUReserved > 0 {
			containerMetrics.CPUUtilized = (containerMetrics.CPUUtilized / containerMetrics.CPUReserved)
		}

		containerResource := containerResource(containerMetadata)
		taskResource.Attributes().ForEach(func(k string, av pdata.AttributeValue) {
			containerResource.Attributes().Upsert(k, av)
		})

		acc.accumulate(convertToOTLPMetrics(ContainerPrefix, containerMetrics, containerResource, timestamp))

		aggregateTaskMetrics(&taskMetrics, containerMetrics)
	}

	// Overwrite Memory limit with task level limit
	if metadata.Limits.Memory != nil {
		taskMetrics.MemoryReserved = *metadata.Limits.Memory
	}

	taskMetrics.CPUReserved = taskMetrics.CPUReserved / CPUsInVCpu

	// Overwrite CPU limit with task level limit
	if metadata.Limits.CPU != nil {
		taskMetrics.CPUReserved = *metadata.Limits.CPU
	}

	// taskMetrics.CPUReserved cannot be zero. In ECS, user needs to set CPU limit
	// at least in one place (either in task level or in container level). If the
	// task level CPULimit is not present, we calculate it from the summation of
	// all container CPU limits.
	taskMetrics.CPUUtilized = ((taskMetrics.CPUUsageInVCPU / taskMetrics.CPUReserved) * 100)

	acc.accumulate(convertToOTLPMetrics(TaskPrefix, taskMetrics, taskResource, timestamp))
}

func (acc *metricDataAccumulator) accumulate(rms pdata.ResourceMetricsSlice) {
	md := pdata.Metrics(rms)

	acc.mds = append(acc.mds, md)
}
