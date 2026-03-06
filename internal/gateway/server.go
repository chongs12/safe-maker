package gateway

import (
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/kitex_gen/safeflow/llmagentservice"
	"github.com/safeflow-project/safeflow/kitex_gen/safeflow/ruleengineservice"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Server struct {
	cfg        *common.Config
	logger     *zap.Logger
	metrics    *common.Metrics
	db         *gorm.DB
	nc         *nats.Conn
	ruleClient ruleengineservice.Client
	llmClient  llmagentservice.Client
	httpClient *http.Client

	simulatorMu      sync.Mutex
	simulatorRunning bool
	simulatorStop    chan struct{}
	simulatorConfig  simulatorConfig
	simulatorCount   int
	simulatorRand    *rand.Rand
}

func NewServer(cfg *common.Config, logger *zap.Logger, metrics *common.Metrics, db *gorm.DB, nc *nats.Conn, ruleClient ruleengineservice.Client, llmClient llmagentservice.Client) *Server {
	return &Server{
		cfg:           cfg,
		logger:        logger,
		metrics:       metrics,
		db:            db,
		nc:            nc,
		ruleClient:    ruleClient,
		llmClient:     llmClient,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
		simulatorRand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "api-gateway"})
	})
	s.registerTracing(r)
	s.registerCORS(r)
	s.registerPublicRoutes(r)
	s.registerAdminRoutes(r)
	return r
}
