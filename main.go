package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/wongnai/xds/internal/config"
	"github.com/wongnai/xds/internal/di"
	"github.com/wongnai/xds/meter"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)

	cfg := config.ParseFlags()

	meter.InstallPromExporter()

	servers, stop, err := di.InitializeServer(context.Background(), cfg)
	if err != nil {
		klog.Fatal(err)
	}

	lis, err := net.Listen("tcp", ":5000") //nolint:gosec // We're not using TLS
	if err != nil {
		klog.Fatal(err)
	}
	go func() {
		err = servers.GrpcServer.Serve(lis)
		if err != nil {
			klog.Fatal(err)
		}
	}()
	klog.Infoln("Server started")

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	<-sigchan

	klog.Infoln("Stopping...")
	stop()
	lis.Close()
	klog.Infoln("Gracefully stopped")
}
