package main

import (
	"context"
	"runtime"

	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	stub "github.com/xunchangguo/kong-operator/pkg/stub"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	printVersion()

	resource := "c2cloud.com/v1alpha1"
	kind := "Kong"
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Fatalf("Failed to get watch namespace: %v", err)
	}
	//TODO 要做kong target的同步处理， 查询出所有crd，和kong做同步
	resyncPeriod := 0 //5
	logrus.Infof("Watching %s, %s, %s, %d", resource, kind, namespace, resyncPeriod)
	sdk.Watch(resource, kind, namespace, resyncPeriod)
	//logrus.Infof("Watching v1, Pod, %s, %d", namespace, resyncPeriod)
	//sdk.Watch("v1", "Pod", namespace, resyncPeriod)
	logrus.Infof("Watching v1, Pod, %s, %d", v1.NamespaceAll, resyncPeriod)
	sdk.Watch("v1", "Pod", v1.NamespaceAll, resyncPeriod)
	sdk.Handle(stub.NewHandler())
	sdk.Run(context.TODO())
}
