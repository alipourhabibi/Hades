package gitaly

// import (
// 	"context"
// 	"fmt"
// 	"sync"
//
// 	gitalypb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb" // Import Gitaly proto package
// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/codes"
// 	"google.golang.org/grpc/status"
// 	"google.golang.org/protobuf/proto" // For deep copying proto messages
// )
//
// // Define interfaces (for testability)
//
// type GitalyServer interface {
// 	// Your Gitaly service methods here, e.g.:
// 	// Commit(ctx context.Context, req *gitalypb.CommitRequest) (*gitalypb.CommitResponse, error)
// 	// ... other Gitaly service methods
// }
//
// type TransactionManager interface {
// 	Begin(ctx context.Context, repo *gitalypb.Repository) (*Transaction, error)
// 	Commit(ctx context.Context, tx *Transaction) error
// 	Abort(ctx context.Context, tx *Transaction) error
// }
//
// type WriteAheadLog interface {
// 	//gitalypb.ChangedPaths
// 	Log(tx *Transaction, changes *gitalypb.ChangedPaths) error // Changes are now Git requests
// 	Apply(repo *gitalypb.Repository) error                     // Applying changes to underlying Git
// }
//
// // Transaction struct
// type Transaction struct {
// 	id      uint64 // Unique transaction ID
// 	repo    *gitalypb.Repository
// 	changes *gitalypb.ChangedPaths // Store changes made in the transaction (Git request)
// 	mu      sync.Mutex             // Protect snapshot creation (if needed)
//
// 	// Add any other transaction-related data here
// }
//
// // In-memory TransactionManager (for demonstration)
// type InMemoryTransactionManager struct {
// 	nextID uint64
// 	wal    WriteAheadLog // Inject WAL implementation
// }
//
// func NewInMemoryTransactionManager(wal WriteAheadLog) *InMemoryTransactionManager {
// 	return &InMemoryTransactionManager{wal: wal}
// }
//
// func (tm *InMemoryTransactionManager) Begin(ctx context.Context, repo *gitalypb.Repository) (*Transaction, error) {
// 	tm.nextID++
// 	tx := &Transaction{
// 		id:   tm.nextID,
// 		repo: repo,
// 	}
//
// 	// Snapshotting logic (if needed - depends on how you interact with Git)
// 	// If you're using gRPC calls to an external Git process, snapshotting might not be necessary.
// 	// If you're directly manipulating Git data, you'll need a snapshot mechanism.
//
// 	return tx, nil
// }
//
// func (tm *InMemoryTransactionManager) Commit(ctx context.Context, tx *Transaction) error {
// 	if err := tm.wal.Log(tx, tx.changes); err != nil { // Log changes
// 		return err
// 	}
// 	if err := tm.wal.Apply(tx.repo); err != nil { // Apply changes to the underlying Git
// 		return err
// 	}
//
// 	// Cleanup (if snapshotting was used)
// 	return nil
// }
//
// func (tm *InMemoryTransactionManager) Abort(ctx context.Context, tx *Transaction) error {
// 	// Cleanup (if snapshotting was used)
// 	return nil
// }
//
// // Example WriteAheadLog (in-memory for demonstration)
// type InMemoryWAL struct {
// 	logs []*gitalypb.ChangedPaths
// }
//
// func (wal *InMemoryWAL) Log(tx *Transaction, changes *gitalypb.ChangedPaths) error {
// 	// Deep copy the changes to avoid modification after logging
// 	copiedChanges := proto.Clone(changes).(*gitalypb.ChangedPaths)
// 	wal.logs = append(wal.logs, copiedChanges)
// 	return nil
// }
//
// func (wal *InMemoryWAL) Apply(repo *gitalypb.Repository) error {
// 	// Here you would use the logged Git requests to update your actual Git data
// 	// (e.g., call `git update-ref`, `git commit-tree`, etc., via gRPC or direct Git interaction).
// 	fmt.Println("Applying changes to repository:", repo.GetRelativePath())
// 	for _, change := range wal.logs {
// 		fmt.Println("Applying change:", change) // Print the change for demonstration
// 	}
// 	return nil
// }
//
// // gRPC Interceptor
// func TransactionInterceptor(tm TransactionManager, gitalyServer GitalyServer) grpc.UnaryServerInterceptor {
// 	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
//
// 		// Extract repository info from request (using Gitaly proto)
// 		repo := extractRepositoryFromRequest(req)
//
// 		tx, err := tm.Begin(ctx, repo)
// 		if err != nil {
// 			return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
// 		}
//
// 		// Store the request (or relevant parts of it) as the changes in the transaction
// 		tx.changes = getChangedPathsFromRequest(req) // Implement this function
//
// 		defer func() { // Ensure transaction cleanup
// 			if r := recover(); r != nil {
// 				tm.Abort(ctx, tx)
// 				panic(r) // Re-panic after aborting
// 			}
// 		}()
//
// 		// Execute handler (passing the transaction context if needed)
// 		resp, err := handler(ctx, req) // No need to rewrite request
//
// 		if err == nil {
// 			if err := tm.Commit(ctx, tx); err != nil {
// 				return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
// 			}
// 		} else {
// 			tm.Abort(ctx, tx)
// 		}
//
// 		return resp, err
// 	}
// }
//
// // Placeholder functions - Replace with your actual implementation
// func extractRepositoryFromRequest(req interface{}) *gitalypb.Repository {
// 	// Your logic to extract Repository object from the request.
// 	// This will likely involve type assertions and accessing fields of your request struct.
// 	// Example (adapt to your needs):
// 	type MyRequest struct {
// 		Repo *gitalypb.Repository
// 		// ... other fields
// 	}
// 	myReq := req.(*MyRequest) // Type assertion
// 	return myReq.Repo
// }
//
// func getChangedPathsFromRequest(req interface{}) *gitalypb.ChangedPaths {
// 	// Extract the Git-related parts of the request.  This will be highly specific to your RPC methods.
// 	// Example (adapt to your needs - this is VERY simplified):
// 	type MyRequest struct {
// 		// ... other fields
// 		GitCommand *gitalypb.ChangedPaths // Hypothetical field containing Git data
// 	}
// 	myReq := req.(*MyRequest)
// 	return myReq.GitCommand
// }
//
// // Example usage:
// func main() {
// 	// Example WAL and Transaction Manager setup
// 	// wal := &InMemoryWAL{}
// 	// tm := NewInMemoryTransactionManager(wal)
//
// 	// Example Gitaly server setup (using the interceptor)
// 	// gitalyServer := &MyGitalyServer{} // Your Gitaly server implementation
// 	// s := grpc.NewServer(grpc.UnaryInterceptor(TransactionInterceptor(tm, gitalyServer)))
// 	// gitalypb.RegisterCommitServiceServer(s, gitalyServer) // Register your Gitaly server with gRPC
// 	// ... register other gRPC services ...
// 	// s.Serve(...)
// }
//
// // Example Gitaly server implementation (replace with your actual implementation)
// type MyGitalyServer struct{}
//
// // Example Gitaly method implementation (replace with your actual methods)
// func (s *MyGitalyServer) Commit(ctx context.Context, req *gitalypb.UserCommitFilesRequest) (*gitalypb.UserCommitFilesResponse, error) {
// 	// Your actual Commit implementation here.
// 	// Access the repository using req.GetRepository()
// 	fmt.Println("Received Commit request for repo:", req.UserCommitFilesRequestPayload)
//
// 	// ... your Git logic here ...
//
// 	return &gitalypb.UserCommitFilesResponse{}, nil // Replace with your actual response
// }
//
// // ... implement other Gitaly server methods ...
