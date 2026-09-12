package gitaly

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/alipourhabibi/Hades/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// StorageService aggregates all Gitaly gRPC sub-service clients.
//
// Every sub-service shares one connection. gRPC multiplexes concurrent RPCs
// over a single HTTP/2 connection, so six connections to one address bought
// nothing and leaked six file descriptors and six connection-management
// goroutines per process, none of which were ever closed.
type StorageService struct {
	CommitService     *CommitService
	BlobService       *BlobService
	OperattionService *OperationService
	RepositoryService *RepositoryService
	DiffService       *DiffService
	TreeService       *TreeService

	conn *grpc.ClientConn
}

// Close releases the shared gRPC connection.
func (s *StorageService) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

// NewStorage dials the Gitaly server once and initialises all sub-service
// clients on that connection.
func NewStorage(c config.Gitaly) (*StorageService, error) {
	creds, err := transportCredentials(c)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(gitalyAddr(c), grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("gitaly: dial: %w", err)
	}

	return &StorageService{
		CommitService:     newCommitService(conn, c),
		OperattionService: newOperationService(conn, c),
		RepositoryService: newRepositoryService(conn, c),
		BlobService:       newBlobService(conn, c),
		DiffService:       newDiffService(conn, c),
		TreeService:       newTreeService(conn, c),
		conn:              conn,
	}, nil
}

// transportCredentials builds the credentials for the Gitaly connection.
// Insecure is still the default, but it is now a stated choice in config rather
// than the only thing the code can do.
func transportCredentials(c config.Gitaly) (credentials.TransportCredentials, error) {
	if !c.TLS {
		return insecure.NewCredentials(), nil
	}

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.ServerNameOverride != "" {
		cfg.ServerName = c.ServerNameOverride
	}
	if c.CACertFile != "" {
		pem, err := os.ReadFile(c.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("gitaly: read caCertFile: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("gitaly: caCertFile %q contains no certificates", c.CACertFile)
		}
		cfg.RootCAs = pool
	}
	return credentials.NewTLS(cfg), nil
}
