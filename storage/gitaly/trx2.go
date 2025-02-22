package gitaly

// import (
// 	"context"
// 	"fmt"
// 	"time"
//
// 	gitalypb "github.com/cpp597455873/gitaly-proto/go/gitalypb"
// 	"google.golang.org/grpc"
// 	"google.golang.org/grpc/credentials"
// 	"google.golang.org/grpc/grpclog"
// 	"google.golang.org/grpc/keepalive"
// 	"google.golang.org/uuid"
// )
//
// type TransactionManager struct {
// 	client     gitalypb.GitalyClient
// 	repository string
// 	mainBranch string
// }
//
// func NewTransactionManager(gitalyAddress string, repository string, mainBranch string) (*TransactionManager, error) {
// 	conn, err := createGitalyConnection(gitalyAddress)
// 	if err != nil {
// 		return nil, err
// 	}
// 	client := gitalypb.NewGitalyClient(conn)
// 	return &TransactionManager{
// 		client:     client,
// 		repository: repository,
// 		mainBranch: mainBranch,
// 	}, nil
// }
//
// func createGitalyConnection(address string) (*grpc.ClientConn, error) {
// 	creds, err := credentials.NewClientTLSFromFile("path/to/cert", "localhost")
// 	if err != nil {
// 		return nil, err
// 	}
// 	opts := []grpc.DialOption{
// 		grpc.WithTransportCredentials(creds),
// 		grpc.WithKeepaliveParams(keepalive.ClientParameters{
// 			Time: 10 * time.Minute,
// 		}),
// 		grpc.WithUnaryInterceptor(logUnaryClient),
// 	}
// 	return grpc.Dial(address, opts...)
// }
//
// func logUnaryClient(
// 	ctx context.Context,
// 	method string,
// 	req, reply interface{},
// 	cc *grpc.ClientConn,
// 	invoker grpc.UnaryInvoker,
// 	opts ...grpc.CallOption,
// ) error {
// 	start := time.Now()
// 	err := invoker(ctx, method, req, reply, cc, opts...)
// 	duration := time.Since(start)
// 	grpclog.Infoln("RPC:", method, "Duration:", duration, "Error:", err)
// 	return err
// }
//
// func (tm *TransactionManager) StartTransaction() (string, error) {
// 	req := &gitalypb.GetBranchRequest{
// 		Repository: &gitalypb.Repository{Name: tm.repository},
// 		Name:       tm.mainBranch,
// 	}
// 	res, err := tm.client.GetBranch(context.Background(), req)
// 	if err != nil {
// 		return "", err
// 	}
// 	currentCommit := res.CommitId
// 	transactionBranch := fmt.Sprintf("transaction-%s", uuid.New().String())
// 	req = &gitalypb.CreateBranchRequest{
// 		Repository: &gitalypb.Repository{Name: tm.repository},
// 		Name:       transactionBranch,
// 		Target:     currentCommit,
// 	}
// 	_, err = tm.client.CreateBranch(context.Background(), req)
// 	if err != nil {
// 		return "", err
// 	}
// 	return transactionBranch, nil
// }
//
// func getCommitOfBranch(client gitalypb.GitalyClient, repository, branch string) (string, error) {
// 	req := &gitalypb.GetBranchRequest{
// 		Repository: &gitalypb.Repository{Name: repository},
// 		Name:       branch,
// 	}
// 	res, err := client.GetBranch(context.Background(), req)
// 	if err != nil {
// 		return "", err
// 	}
// 	return res.CommitId, nil
// }
//
// func (tm *TransactionManager) CreateCommit(transactionID string, message string, files []*gitalypb.File) (string, error) {
// 	parentCommit, err := getCommitOfBranch(tm.client, tm.repository, transactionID)
// 	if err != nil {
// 		return "", err
// 	}
// 	req := &gitalypb.CreateCommitRequest{
// 		Repository: &gitalypb.Repository{Name: tm.repository},
// 		Author: &gitalypb.CommitAuthor{
// 			Name:  "Transaction Manager",
// 			Email: "transaction@manager.com",
// 		},
// 		Committer: &gitalypb.CommitAuthor{
// 			Name:  "Transaction Manager",
// 			Email: "transaction@manager.com",
// 		},
// 		Message:   message,
// 		ParentIds: []string{parentCommit},
// 		Tree: &gitalypb.Tree{
// 			Entries: files,
// 		},
// 	}
// 	res, err := tm.client.CreateCommit(context.Background(), req)
// 	if err != nil {
// 		return "", err
// 	}
// 	req = &gitalypb.UpdateRefRequest{
// 		Repository:  &gitalypb.Repository{Name: tm.repository},
// 		Ref:         &gitalypb.Ref{Name: transactionID},
// 		NewRevision: res.CommitId,
// 		Force:       true,
// 	}
// 	_, err = tm.client.UpdateRef(context.Background(), req)
// 	if err != nil {
// 		return "", err
// 	}
// 	return res.CommitId, nil
// }
//
// func (tm *TransactionManager) CommitTransaction(transactionID string) error {
// 	req := &gitalypb.MergingMergeRequest{
// 		Repository:   &gitalypb.Repository{Name: tm.repository},
// 		SourceBranch: transactionID,
// 		TargetBranch: tm.mainBranch,
// 	}
// 	_, err := tm.client.MergingMerge(context.Background(), req)
// 	if err != nil {
// 		return err
// 	}
// 	req = &gitalypb.DeleteBranchRequest{
// 		Repository: &gitalypb.Repository{Name: tm.repository},
// 		Name:       transactionID,
// 	}
// 	_, err = tm.client.DeleteBranch(context.Background(), req)
// 	return err
// }
//
// func (tm *TransactionManager) RollbackTransaction(transactionID string) error {
// 	req := &gitalypb.DeleteBranchRequest{
// 		Repository: &gitalypb.Repository{Name: tm.repository},
// 		Name:       transactionID,
// 	}
// 	_, err := tm.client.DeleteBranch(context.Background(), req)
// 	return err
// }
