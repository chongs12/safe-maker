package common

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 定义系统指标
type Metrics struct {
	// HTTP 请求相关指标
	HTTPRequestTotal *prometheus.CounterVec
	HTTPDuration     *prometheus.HistogramVec
	HTTPInFlight     *prometheus.GaugeVec

	// 审核服务相关指标
	AuditRequestsTotal *prometheus.CounterVec
	AuditDuration      *prometheus.HistogramVec
	AuditActionsTotal  *prometheus.CounterVec
	AuditRiskScores    *prometheus.HistogramVec

	// LLM 调用相关指标
	LLMCallsTotal  *prometheus.CounterVec
	LLMDuration    *prometheus.HistogramVec
	LLMErrorsTotal *prometheus.CounterVec

	// 规则引擎相关指标
	RuleMatchesTotal   *prometheus.CounterVec
	RuleEngineDuration *prometheus.HistogramVec

	// RAG 检索相关指标
	RAGQueriesTotal *prometheus.CounterVec
	RAGDuration     *prometheus.HistogramVec
	RAGResultsCount *prometheus.HistogramVec

	// 系统资源指标
	GoGoroutines prometheus.GaugeFunc
	GoThreads    prometheus.GaugeFunc
	GoAllocBytes prometheus.GaugeFunc
}

var GlobalMetrics *Metrics

// InitMetrics 初始化全局指标
func InitMetrics(serviceName string) *Metrics {
	m := &Metrics{
		// HTTP 请求指标
		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "http_requests_total",
				Help:        "Total number of HTTP requests",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"method", "endpoint", "status"},
		),
		HTTPDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "http_request_duration_seconds",
				Help:        "HTTP request duration in seconds",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		HTTPInFlight: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name:        "http_requests_in_flight",
				Help:        "Number of HTTP requests currently being served",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"method", "endpoint"},
		),

		// 审核服务指标
		AuditRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "audit_requests_total",
				Help:        "Total number of audit requests",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"source", "scene"},
		),
		AuditDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "audit_duration_seconds",
				Help:        "Audit processing duration in seconds",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30},
			},
			[]string{"source", "scene"},
		),
		AuditActionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "audit_actions_total",
				Help:        "Total number of audit actions taken",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"action", "source", "scene"},
		),
		AuditRiskScores: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "audit_risk_scores",
				Help:        "Distribution of risk scores",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0, 0.1, 0.3, 0.5, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"source", "scene"},
		),

		// LLM 调用指标
		LLMCallsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "llm_calls_total",
				Help:        "Total number of LLM calls",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"model", "node"},
		),
		LLMDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "llm_call_duration_seconds",
				Help:        "LLM call duration in seconds",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0.5, 1, 2, 5, 10, 20, 30},
			},
			[]string{"model", "node"},
		),
		LLMErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "llm_errors_total",
				Help:        "Total number of LLM errors",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"model", "node", "error_type"},
		),

		// 规则引擎指标
		RuleMatchesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "rule_matches_total",
				Help:        "Total number of rule matches",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"category", "action"},
		),
		RuleEngineDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "rule_engine_duration_seconds",
				Help:        "Rule engine processing duration",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
			},
			[]string{"engine_type"},
		),

		// RAG 检索指标
		RAGQueriesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "rag_queries_total",
				Help:        "Total number of RAG queries",
				ConstLabels: prometheus.Labels{"service": serviceName},
			},
			[]string{"collection"},
		),
		RAGDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "rag_query_duration_seconds",
				Help:        "RAG query duration in seconds",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10},
			},
			[]string{"collection"},
		),
		RAGResultsCount: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "rag_results_count",
				Help:        "Number of results returned by RAG",
				ConstLabels: prometheus.Labels{"service": serviceName},
				Buckets:     []float64{0, 1, 2, 3, 5, 10, 20},
			},
			[]string{"collection"},
		),
	}

	// Go 运行时指标由 Prometheus 默认提供，无需重复注册
	// m.GoGoroutines = promauto.NewGaugeFunc(...)
	// m.GoThreads = promauto.NewGaugeFunc(...)
	// m.GoAllocBytes = promauto.NewGaugeFunc(...)

	GlobalMetrics = m
	return m
}

// HTTPMiddleware HTTP 中间件，收集请求指标
func (m *Metrics) HTTPMiddleware() func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			method := r.Method
			endpoint := r.URL.Path

			// 在飞行请求数
			m.HTTPInFlight.WithLabelValues(method, endpoint).Inc()
			defer m.HTTPInFlight.WithLabelValues(method, endpoint).Dec()

			// 执行下一个处理器
			next(w, r)

			// 记录总请求数和耗时
			duration := time.Since(start).Seconds()
			m.HTTPRequestTotal.WithLabelValues(method, endpoint, "200").Inc() // 简化处理
			m.HTTPDuration.WithLabelValues(method, endpoint).Observe(duration)
		}
	}
}
