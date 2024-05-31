// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package prometheus

import (
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"

	"github.com/aws/amazon-cloudwatch-agent/translator/translate/logs/metrics_collected/prometheus"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/common"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/exporter/awscloudwatch"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/exporter/awsemf"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/exporter/prometheusremotewrite"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/extension/agenthealth"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/extension/sigv4auth"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/processor/batchprocessor"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/processor/ec2taggerprocessor"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/processor/rollupprocessor"
	"github.com/aws/amazon-cloudwatch-agent/translator/translate/otel/receiver/adapter"
)

const (
	pipelineName = "prometheus"
)

var (
	metricsDestinationsKey = common.ConfigKey(common.MetricsKey, common.MetricsDestinationsKey)
)

type translator struct {
	section string
}

type Option func(any)

func WithSection(section string) Option {
	return func(a any) {
		if t, ok := a.(*translator); ok {
			t.section = section
		}
	}
}

var _ common.Translator[*common.ComponentTranslators] = (*translator)(nil)

func NewTranslator(opts ...Option) common.Translator[*common.ComponentTranslators] {
	t := &translator{
		section: common.LogsKey,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *translator) ID() component.ID {
	return component.NewIDWithName(component.DataTypeMetrics, pipelineName+t.section)
}

// Translate creates a pipeline for prometheus if the logs.metrics_collected.prometheus
// section is present.
func (t *translator) Translate(conf *confmap.Conf) (*common.ComponentTranslators, error) {
	key := common.ConfigKey(t.section, common.MetricsCollectedKey, common.PrometheusKey)
	if conf == nil || !conf.IsSet(key) {
		return nil, &common.MissingKeyError{ID: t.ID(), JsonKey: key}
	}

	translators := common.ComponentTranslators{
		Receivers:  common.NewTranslatorMap(adapter.NewTranslator(prometheus.SectionKey, key, time.Minute)),
		Processors: common.NewTranslatorMap[component.Config](),
		Exporters:  common.NewTranslatorMap[component.Config](),
		Extensions: common.NewTranslatorMap[component.Config](),
	}

	if common.MetricsKey == t.section {
		if !conf.IsSet(metricsDestinationsKey) || conf.IsSet(common.ConfigKey(metricsDestinationsKey, "cloudwatch")) {
			translators.Exporters.Set(awscloudwatch.NewTranslator())
			translators.Extensions.Set(agenthealth.NewTranslator(component.DataTypeMetrics, []string{agenthealth.OperationPutMetricData}))
		}
		if conf.IsSet(metricsDestinationsKey) && conf.IsSet(common.ConfigKey(metricsDestinationsKey, common.AMPKey)) {
			translators.Exporters.Set(prometheusremotewrite.NewTranslatorWithName(pipelineName))
			translators.Processors.Set(batchprocessor.NewTranslatorWithNameAndSection(pipelineName, t.section))
			if conf.IsSet(common.MetricsAggregationDimensionsKey) {
				translators.Processors.Set(rollupprocessor.NewTranslator())
			}
			translators.Extensions.Set(sigv4auth.NewTranslator())
		}
		if conf.IsSet(common.ConfigKey(common.MetricsKey, common.AppendDimensionsKey)) {
			translators.Processors.Set(ec2taggerprocessor.NewTranslator())
		}
	} else if common.LogsKey == t.section {
		translators.Exporters.Set(awsemf.NewTranslatorWithName(pipelineName))
		translators.Processors.Set(batchprocessor.NewTranslatorWithNameAndSection(pipelineName, t.section))
		translators.Extensions.Set(agenthealth.NewTranslator(component.DataTypeLogs, []string{agenthealth.OperationPutLogEvents}))
	}

	return &translators, nil
}
