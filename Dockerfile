# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build API Gateway
RUN go build -o /bin/api-gateway ./cmd/api-gateway

# Build Rule Engine
RUN go build -o /bin/rule-engine ./cmd/rule-engine

# Build LLM Agent
RUN go build -o /bin/llm-agent ./cmd/llm-agent

# Build Audit Service
RUN go build -o /bin/audit-service ./cmd/audit-service

# Final stages
FROM alpine:latest AS api-gateway
WORKDIR /app
COPY --from=builder /bin/api-gateway .
CMD ["./api-gateway"]

FROM alpine:latest AS rule-engine
WORKDIR /app
COPY --from=builder /bin/rule-engine .
CMD ["./rule-engine"]

FROM alpine:latest AS llm-agent
WORKDIR /app
COPY --from=builder /bin/llm-agent .
CMD ["./llm-agent"]

FROM alpine:latest AS audit-service
WORKDIR /app
COPY --from=builder /bin/audit-service .
CMD ["./audit-service"]
