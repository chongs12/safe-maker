package common

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// TracerProvider 全局 tracer provider
var TracerProvider *sdktrace.TracerProvider

// InitTracing 初始化 OpenTelemetry 链路追踪
func InitTracing(serviceName, jaegerEndpoint string) error {
	// 创建资源
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return fmt.Errorf("创建资源失败: %w", err)
	}

	// 创建 Jaeger exporter
	var exporter sdktrace.SpanExporter
	if jaegerEndpoint != "" {
		exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(
			jaeger.WithEndpoint(jaegerEndpoint),
		))
		if err != nil {
			log.Printf("Jaeger exporter 创建失败，使用 stdout: %v", err)
			exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
			if err != nil {
				return fmt.Errorf("stdout exporter 创建失败: %w", err)
			}
		}
	} else {
		// 使用标准输出作为备选
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return fmt.Errorf("stdout exporter 创建失败: %w", err)
		}
	}

	// 创建 Tracer Provider
	TracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// 设置全局 Tracer Provider
	otel.SetTracerProvider(TracerProvider)

	// 设置 propagator (用于跨服务传递 trace context)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Printf("链路追踪初始化完成，服务: %s", serviceName)
	return nil
}

// GetTracer 获取 tracer 实例
func GetTracer() trace.Tracer {
	return otel.GetTracerProvider().Tracer("safeflow")
}

// StartSpan 开始一个新的 span
func StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := GetTracer()
	return tracer.Start(ctx, spanName, opts...)
}

// CloseTracing 关闭 tracing，确保所有 spans 被导出
func CloseTracing() error {
	if TracerProvider != nil {
		return TracerProvider.Shutdown(context.Background())
	}
	return nil
}

// TraceMiddleware HTTP 中间件，为每个请求创建根 span
func TraceMiddleware(serviceName string) func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 从请求头中提取 trace context
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// 创建根 span
			ctx, span := StartSpan(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path))
			defer span.End()

			// 添加请求相关信息到 span
			span.SetAttributes(
				semconv.HTTPMethodKey.String(r.Method),
				semconv.HTTPURLKey.String(r.URL.String()),
				semconv.HTTPUserAgentKey.String(r.UserAgent()),
				semconv.NetSockPeerAddrKey.String(r.RemoteAddr),
			)

			// 将 trace context 注入到响应头中
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

			// 执行下一个处理器
			next(w, r.WithContext(ctx))

			// TODO: 从 ResponseWriter 包装器中获取状态码
			// span.SetAttributes(semconv.HTTPStatusCodeKey.Int(statusCode))
		}
	}
}
