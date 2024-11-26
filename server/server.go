package server

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/grpcreflect"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/storage/db"
	"github.com/alipourhabibi/Hades/utils/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// SchemaRegistryServer is the API server for SchemaRegistry
type SchemaRegistryServer struct {
	logger *log.LoggerWrapper

	db *db.DBs

	listenPort int

	serverSet *SchemaRegistryServerSet
}

// SchemaRegistryServerSet holds all the server for schema registry
type SchemaRegistryServerSet struct {
}

type SchemaRegistryConfiguration func(*SchemaRegistryServer) error

// WithLogger injects the logger to the SchemaRegistryServer
func WithLogger(logger *log.LoggerWrapper) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.logger = logger
		return nil
	}
}

func defaultSchmeaRegistryServer() *SchemaRegistryServer {
	return &SchemaRegistryServer{
		listenPort: 50051,
		logger:     log.DefaultLogger(),
	}
}

// WithDB injects the database into the SchemaRegistryServer
func WithDB(db *db.DBs) SchemaRegistryConfiguration {
	return func(ss *SchemaRegistryServer) error {
		ss.db = db
		return nil
	}
}

// NewServer returns a new instance of the Schema Registry API server
func NewServer(ctx context.Context, c *config.Config, cfgs ...SchemaRegistryConfiguration) (*SchemaRegistryServer, error) {

	ss := defaultSchmeaRegistryServer()

	if c.Server.ListenPort != 0 {
		ss.listenPort = c.Server.ListenPort
	}

	// replace configs with the given configs
	for _, cfg := range cfgs {
		err := cfg(ss)
		if err != nil {
			return nil, err
		}
	}

	return ss, nil
}

// Run runs the SchmeaRegistry server
func (s *SchemaRegistryServer) Run(ctx context.Context, cancel context.CancelFunc) {

	var err error
	// this should be another place?
	s.serverSet, err = newSchemaRegistryServerSet(s)
	if err != nil {
		s.logger.Error("Failed to start server", "port", s.listenPort, "error", err)
		cancel()
		return
	}

	mux := s.newServerMux()

	s.logger.Info("StartingServer...", "port", s.listenPort)
	err = http.ListenAndServe(fmt.Sprintf(":%d", s.listenPort), h2c.NewHandler(mux, &http2.Server{}))
	if err != nil {
		s.logger.Error("Failed to start server", "port", s.listenPort, "error", err)
		cancel()
		return
	}
}

// newSchemaRegistryServerSet creates the SchemaRegistryServerSet from the attributes in SchemaRegistryServer
func newSchemaRegistryServerSet(_ *SchemaRegistryServer) (*SchemaRegistryServerSet, error) {

	serverSet := &SchemaRegistryServerSet{}

	return serverSet, nil
}

// newServerMux creates the server mux based on the attributes in SchemaRegistryServer
func (s *SchemaRegistryServer) newServerMux() *http.ServeMux {
	mux := http.NewServeMux()

	reflector := grpcreflect.NewStaticReflector(
	// Register all your services with the reflector
	)
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	return mux
}
