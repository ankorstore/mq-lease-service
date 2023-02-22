package metrics

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics interface {
	GetFactory() promauto.Factory
	GetHTTPHandler() http.Handler
	AddDefaultCollectors()
	NewCounter(opts prometheus.CounterOpts) prometheus.Counter
	NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec
	NewGauge(opts prometheus.GaugeOpts) prometheus.Gauge
	NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec
	NewSummary(opts prometheus.SummaryOpts) prometheus.Summary
	NewSummaryVec(opts prometheus.SummaryOpts, labelNames []string) *prometheus.SummaryVec
	NewHistogram(opts prometheus.HistogramOpts) prometheus.Histogram
	NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec
}

type NewOpts struct {
	AppName        string
	ConstLabels    map[string]string
	PromRegisterer prometheus.Registerer
	PromGatherer   prometheus.Gatherer
}

func New(opts NewOpts) Metrics {
	if opts.ConstLabels == nil {
		opts.ConstLabels = make(map[string]string)
	}
	if opts.PromRegisterer == nil {
		opts.PromRegisterer = prometheus.DefaultRegisterer
	}
	if opts.PromGatherer == nil {
		opts.PromGatherer = prometheus.DefaultGatherer
	}

	return &metricsImpl{
		promRegisterer: opts.PromRegisterer,
		promGatherer:   opts.PromGatherer,
		appName:        sanitizeName(opts.AppName),
		constLabels:    opts.ConstLabels,
	}
}

type metricsImpl struct {
	promRegisterer prometheus.Registerer
	promGatherer   prometheus.Gatherer
	appName        string
	constLabels    map[string]string
}

func (m *metricsImpl) GetFactory() promauto.Factory {
	return promauto.With(m.promRegisterer)
}

func (m *metricsImpl) AddDefaultCollectors() {
	m.promRegisterer.MustRegister(collectors.NewBuildInfoCollector())
	m.promRegisterer.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{Matcher: regexp.MustCompile("/.*")}),
	))
}

func (m *metricsImpl) GetHTTPHandler() http.Handler {
	return promhttp.InstrumentMetricHandler(
		m.promRegisterer,
		promhttp.HandlerFor(m.promGatherer, promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		}),
	)
}

func (m *metricsImpl) NewCounter(opts prometheus.CounterOpts) prometheus.Counter {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewCounter(opts)
}

func (m *metricsImpl) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewCounterVec(opts, m.mergeLabelsNames(labelNames))
}

func (m *metricsImpl) NewGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewGauge(opts)
}

func (m *metricsImpl) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewGaugeVec(opts, m.mergeLabelsNames(labelNames))
}

func (m *metricsImpl) NewSummary(opts prometheus.SummaryOpts) prometheus.Summary {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewSummary(opts)
}

func (m *metricsImpl) NewSummaryVec(opts prometheus.SummaryOpts, labelNames []string) *prometheus.SummaryVec {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewSummaryVec(opts, m.mergeLabelsNames(labelNames))
}

func (m *metricsImpl) NewHistogram(opts prometheus.HistogramOpts) prometheus.Histogram {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewHistogram(opts)
}

func (m *metricsImpl) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	opts.Namespace = m.appName
	opts.ConstLabels = m.mergeLabels(opts.ConstLabels)
	return m.GetFactory().NewHistogramVec(opts, m.mergeLabelsNames(labelNames))
}

func GetDefaultDurationBuckets() []float64 {
	return []float64{
		0.000000001, // 1ns
		0.000000002,
		0.000000005,
		0.00000001, // 10ns
		0.00000002,
		0.00000005,
		0.0000001, // 100ns
		0.0000002,
		0.0000005,
		0.000001, // 1µs
		0.000002,
		0.000005,
		0.00001, // 10µs
		0.00002,
		0.00005,
		0.0001, // 100µs
		0.0002,
		0.0005,
		0.001, // 1ms
		0.002,
		0.005,
		0.01, // 10ms
		0.02,
		0.05,
		0.1, // 100 ms
		0.2,
		0.5,
		1.0, // 1s
		2.0,
		5.0,
		10.0, // 10s
		15.0,
		20.0,
		30.0,
	}
}

func (m *metricsImpl) mergeLabels(providedLabels prometheus.Labels) prometheus.Labels {
	labels := make(prometheus.Labels)
	for k, v := range m.constLabels {
		labels[k] = v
	}
	for k, v := range providedLabels {
		labels[k] = v
	}
	return labels
}

func (m *metricsImpl) mergeLabelsNames(providedLabelNames []string) []string {
	var labelNames []string
	keys := map[string]struct{}{}

	for k := range m.constLabels {
		keys[k] = struct{}{}
		labelNames = append(labelNames, k)
	}

	for _, k := range providedLabelNames {
		if _, ok := keys[k]; !ok {
			labelNames = append(labelNames, k)
		}
	}

	return labelNames
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, ".", "")
	name = strings.ToLower(name)
	name = strings.Trim(name, " -_.")
	return name
}
