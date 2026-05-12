//go:build wireinject

package di

import (
	"context"

	"github.com/google/wire"
	"github.com/wongnai/xds/debug"
	"github.com/wongnai/xds/internal/config"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

type Servers struct {
	DevServer

	_GrpcHealth SideEffectGrpcHealthRegistered
	_Reflection SideEffectGrpcReflectionRegistered
	_Channelz   SideEffectGrpcChannelzRegistered

	DebugServer *debug.Server
}

type DevServer struct {
	_Xds       XdsAllSideEffects
	GrpcServer *grpc.Server
}

func InitializeServer(ctx context.Context, cfg config.Config) (Servers, func(), error) {
	wire.Build(
		KubernetesSet,
		GrpcSet,
		K8sXdsSet,
		XdsSet,
		ProvideOtelGrpcServerOptions,
		wire.Struct(new(DevServer), "*"),
		wire.Struct(new(Servers), "*"),
	)
	return Servers{}, nil, nil
}

func InitializeTestServer(ctx context.Context, kubeClient kubernetes.Interface, cfg config.Config) (TestServer, func(), error) {
	wire.Build(
		GrpcSet,
		K8sXdsSet,
		XdsSet,
		TestSet,
		wire.Struct(new(DevServer), "*"),
		wire.Struct(new(TestServer), "*"),
	)

	return TestServer{}, nil, nil
}
