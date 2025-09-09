package grpc_clients

import (
	userPb "clarity-ai/api/proto/user"
	videoPb "clarity-ai/api/proto/video"
	"clarity-ai/internal/config"
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ClientManager struct {
	userConn    *grpc.ClientConn
	videoConn   *grpc.ClientConn
	UserClient  userPb.UserServiceClient
	VideoClient videoPb.VideoAnalyzerServiceClient
}

func NewClientManager(cfg *config.Config) (*ClientManager, error) {
	cm := &ClientManager{}

	if err := cm.initializeUserClient(cfg); err != nil {
		return nil, fmt.Errorf("failed to initilize user client %w", err)
	}

	if err := cm.initilizeVideoClient(cfg); err != nil {
		return nil, fmt.Errorf("failed to initilize video client %w", err)
	}

	return cm, nil
}

func (cm *ClientManager) initializeUserClient(cfg *config.Config) error {
	addr := cm.getServiceAddress("user-service", cfg.Server.GRPCPort, cfg)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	cm.userConn = conn
	cm.UserClient = userPb.NewUserServiceClient(conn)
	return nil
}

func (cm *ClientManager) initilizeVideoClient(cfg *config.Config) error {
	addr := cm.getServiceAddress("video-service", cfg.Server.VideoGRPCPort, cfg)

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil
	}

	cm.videoConn = conn
	cm.VideoClient = videoPb.NewVideoAnalyzerServiceClient(conn)
	return nil
}

func (cm *ClientManager) getServiceAddress(serviceName, port string, cfg *config.Config) string {
	if cfg.Server.Environment == "development" {
		return "localhost:" + port
	}
	return serviceName + ":" + port
}

func (cm *ClientManager) Close() {
	if cm.userConn != nil {
		cm.userConn.Close()
	}
	if cm.videoConn != nil {
		cm.videoConn.Close()
	}
}

func (cm *ClientManager) HealthCheck(ctx context.Context) error {
	return nil
}
